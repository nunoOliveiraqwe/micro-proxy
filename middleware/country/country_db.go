package country

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/oschwald/maxminddb-golang/v2"
	"go.uber.org/zap"
)

// ListMode determines whether the country/continent list acts as an allow list or a block list.
type ListMode int

const (
	// AllowList only permits traffic from countries/continents in the list.
	AllowList ListMode = iota
	// BlockList only blocks traffic from countries/continents in the list.
	BlockList
)

type clientEntry struct {
	logger        *zap.Logger
	ip            string
	IsAllowed     bool
	lastSeen      time.Time
	countryCode   string
	continentCode string
}

func (c *clientEntry) Touch() {
	c.logger.Info("Touching client entry for ", zap.String("IP", c.ip))
	c.lastSeen = time.Now()
}

func (c *clientEntry) GetLastReadAt() time.Time {
	return c.lastSeen
}

type Filter struct {
	mu                 sync.RWMutex
	db                 *maxminddb.Reader
	loader             DbLoader
	refreshInterval    time.Duration
	clientCache        *util.Cache[*clientEntry]
	countryMode        ListMode
	countries          map[string]byte
	hasCountryList     bool
	continentMode      ListMode
	continents         map[string]byte
	hasContinentList   bool
	countryFieldPath   []string // pre-split field path, e.g. ["country_code"] or ["country", "iso_code"]
	continentFieldPath []string
	onUnknown          bool // true = allow unknown, false = block unknown
}

func splitFieldPath(field string) []string {
	if field == "" {
		return nil
	}
	if !strings.Contains(field, ".") {
		return []string{field}
	}
	return strings.Split(field, ".")
}

func NewFilter(ctx context.Context, cacheOpts *util.CacheOptions, loader DbLoader, countryMode ListMode, countryCodes []string, continentMode ListMode, continentCodes []string, refreshInterval time.Duration, countryField string, continentField string, onUnknown bool) (*Filter, error) {
	hasCountryList := len(countryCodes) > 0
	hasContinentList := len(continentCodes) > 0

	if !hasCountryList && !hasContinentList {
		return nil, fmt.Errorf("at least one of country-list or continent-list must be specified")
	}

	zap.S().Info("Creating new country db")
	b, err := loader.load()
	if err != nil {
		zap.S().Errorf("Failed to load country db, error: %v", err)
		return nil, err
	}
	zap.S().Debugf("Reading country database")
	reader, err := maxminddb.OpenBytes(b)
	if err != nil {
		zap.S().Errorf("Failed to read country db, error: %v", err)
		return nil, err
	}
	cache, err := util.NewCache[*clientEntry](cacheOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create client cache: %w", err)
	}

	countrySet := make(map[string]byte, len(countryCodes))
	for _, code := range countryCodes {
		countrySet[code] = byte(0)
	}

	continentSet := make(map[string]byte, len(continentCodes))
	for _, code := range continentCodes {
		continentSet[code] = byte(0)
	}

	zap.S().Infof("Country filter: country %s mode with %d entries, continent %s mode with %d entries, on-unknown=%v",
		modeString(countryMode), len(countrySet), modeString(continentMode), len(continentSet), onUnknown)
	f := &Filter{
		clientCache:        cache,
		db:                 reader,
		loader:             loader,
		refreshInterval:    refreshInterval,
		countryMode:        countryMode,
		countries:          countrySet,
		hasCountryList:     hasCountryList,
		continentMode:      continentMode,
		continents:         continentSet,
		hasContinentList:   hasContinentList,
		countryFieldPath:   splitFieldPath(countryField),
		continentFieldPath: splitFieldPath(continentField),
		onUnknown:          onUnknown,
	}

	if refreshInterval > 0 && loader.isRefreshable() {
		f.startRefresh(ctx)
	}

	return f, nil
}

func modeString(m ListMode) string {
	switch m {
	case AllowList:
		return "AllowList"
	case BlockList:
		return "BlockList"
	default:
		return "Unknown"
	}
}

func (c *Filter) startRefresh(ctx context.Context) {
	zap.S().Infof("Starting country DB refresh goroutine with interval %s", c.refreshInterval)
	ticker := time.NewTicker(c.refreshInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				zap.S().Infof("Country DB refresh goroutine stopped")
				return
			case <-ticker.C:
				c.reloadDB()
			}
		}
	}()
}

func (c *Filter) reloadDB() {
	zap.S().Info("Reloading country database from remote source")
	b, err := c.loader.load()
	if err != nil {
		zap.S().Errorf("Failed to reload country db: %v. Keeping existing database.", err)
		return
	}
	reader, err := maxminddb.OpenBytes(b)
	if err != nil {
		zap.S().Errorf("Failed to parse reloaded country db: %v. Keeping existing database.", err)
		return
	}

	c.mu.Lock()
	oldDB := c.db
	c.db = reader
	c.mu.Unlock()

	if oldDB != nil {
		if err := oldDB.Close(); err != nil {
			zap.S().Warnf("Failed to close old country db reader: %v", err)
		}
	}
	zap.S().Info("Country database reloaded successfully")
}

func (c *Filter) IsFromAllowedCountry(logger *zap.Logger, r *http.Request, ip netip.Addr) bool {
	entry, err := c.clientCache.GetValue(ip.String())
	if err != nil && errors.Is(err, util.ErrCacheMiss) {
		entry = c.lookupIPAndCacheValue(logger, r, ip)
	}
	if entry == nil {
		logger.Info("Request decision by on-unknown: cannot determine country",
			zap.Bool("onUnknown", c.onUnknown))
		if !c.onUnknown {
			metrics.CreateAndAddBlockInfo(r, "country-block", "blocked due to unknown country")
		}
		return c.onUnknown
	}
	ctx := context.WithValue(r.Context(), ctxkeys.CountryCode, entry.countryCode)
	ctx = context.WithValue(ctx, ctxkeys.ContinentCode, entry.continentCode)
	*r = *r.WithContext(ctx)

	if !entry.IsAllowed {
		metrics.CreateAndAddBlockInfo(r, "country-block", fmt.Sprintf("blocked country %s, continent %s", entry.countryCode, entry.continentCode))
	}
	return entry.IsAllowed
}

// isAllowed evaluates whether a request should be allowed based on the
// configured country and continent policies.
//
// Policy evaluation order:
//   - If only country-list is configured, the country policy alone decides.
//   - If only continent-list is configured, the continent policy alone decides.
//   - If both are configured, the continent policy is the broad/base policy
//     and the country policy acts as an exception override:
//     1. Check the country list first. If the country code is found in the list,
//     the country-list-mode decides immediately (allow → allow, block → block).
//     2. If the country code is NOT found in the list, the continent policy applies.
//
// When the resolved code is empty (unknown), on-unknown determines the outcome.
func (c *Filter) isAllowed(logger *zap.Logger, r *http.Request, countryCode string, continentCode string) bool {
	bothConfigured := c.hasCountryList && c.hasContinentList

	if bothConfigured {
		// Country acts as an exception to the continent base policy.
		_, countryFound := c.countries[countryCode]
		if countryFound {
			// Country matched the list – the country-list-mode decides.
			switch c.countryMode {
			case AllowList:
				logger.Info("Request ALLOWED: country override – country is in allow-list",
					zap.String("country", countryCode), zap.String("continent", continentCode))
				return true
			case BlockList:
				logger.Info("Request BLOCKED: country override – country is in block-list",
					zap.String("country", countryCode), zap.String("continent", continentCode))
				metrics.CreateAndAddBlockInfo(r, "country-block", fmt.Sprintf("country %s blocked", countryCode))
				return false
			}
		}
		// Country did not match – fall through to the continent base policy.
		allowed := c.evalList(c.continents, c.continentMode, continentCode)
		if allowed {
			logger.Info("Request ALLOWED: no country override, continent policy permits",
				zap.String("country", countryCode), zap.String("continent", continentCode),
				zap.String("continentMode", modeString(c.continentMode)))
		} else {
			logger.Info("Request BLOCKED: no country override, continent policy denies",
				zap.String("country", countryCode), zap.String("continent", continentCode),
				zap.String("continentMode", modeString(c.continentMode)))
			metrics.CreateAndAddBlockInfo(r, "country-block", fmt.Sprintf("continent %s blocked", continentCode))
		}
		return allowed
	}

	// Only one of the two is configured.
	if c.hasCountryList {
		if countryCode == "" {
			logger.Info("Request decision by on-unknown: country code is empty",
				zap.Bool("onUnknown", c.onUnknown))
			return c.onUnknown
		}
		allowed := c.evalList(c.countries, c.countryMode, countryCode)
		if allowed {
			logger.Info("Request ALLOWED: country-only policy permits",
				zap.String("country", countryCode), zap.String("countryMode", modeString(c.countryMode)))
		} else {
			logger.Info("Request BLOCKED: country-only policy denies",
				zap.String("country", countryCode), zap.String("countryMode", modeString(c.countryMode)))
		}
		return allowed
	}

	// hasContinentList only
	if continentCode == "" {
		logger.Info("Request decision by on-unknown: continent code is empty",
			zap.Bool("onUnknown", c.onUnknown))
		return c.onUnknown
	}
	allowed := c.evalList(c.continents, c.continentMode, continentCode)
	if allowed {
		logger.Info("Request ALLOWED: continent-only policy permits",
			zap.String("continent", continentCode), zap.String("continentMode", modeString(c.continentMode)))
	} else {
		logger.Info("Request BLOCKED: continent-only policy denies",
			zap.String("continent", continentCode), zap.String("continentMode", modeString(c.continentMode)))
	}
	return allowed
}

// evalList checks whether a code is present in the set and applies the list mode.
func (c *Filter) evalList(set map[string]byte, mode ListMode, code string) bool {
	if code == "" {
		return c.onUnknown
	}
	_, found := set[code]
	switch mode {
	case AllowList:
		return found
	case BlockList:
		return !found
	default:
		return false
	}
}
func (c *Filter) lookupIPAndCacheValue(logger *zap.Logger, r *http.Request, ip netip.Addr) *clientEntry {
	c.mu.RLock()
	result := c.db.Lookup(ip)
	c.mu.RUnlock()

	if result.Found() {
		var raw map[string]any
		if err := result.Decode(&raw); err != nil {
			logger.Error("Failed to decode country db result for IP", zap.String("IP", ip.String()), zap.Error(err))
			return nil
		}
		logger.Info("Raw DB record", zap.Any("record", raw))

		countryCode := extractFieldByPath(raw, c.countryFieldPath)
		continentCode := extractFieldByPath(raw, c.continentFieldPath)

		logger.Debug("Resolved geo codes for IP",
			zap.String("IP", ip.String()),
			zap.String("Country", countryCode),
			zap.String("Continent", continentCode),
		)

		isAllowed := c.isAllowed(logger, r, countryCode, continentCode)

		entry := &clientEntry{
			logger:        logger,
			ip:            ip.String(),
			IsAllowed:     isAllowed,
			lastSeen:      time.Now(),
			countryCode:   countryCode,
			continentCode: continentCode,
		}
		c.clientCache.CacheValue(ip.String(), entry)
		return entry
	}
	return nil
}

// extractFieldByPath traverses a nested map using a pre-split field path
// and returns the string value at that location, or an empty string if not found.
// For a single-element path like ["country_code"], it does a direct map lookup.
// For multi-element paths like ["country", "iso_code"], it traverses nested maps.
func extractFieldByPath(raw map[string]any, path []string) string {
	if len(path) == 0 {
		return ""
	}
	// Fast path: single key, no nesting (e.g. "country_code")
	if len(path) == 1 {
		if v, ok := raw[path[0]]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	// Nested path: walk through intermediate maps
	var current any = raw
	for _, part := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}
	if s, ok := current.(string); ok {
		return s
	}
	return ""
}
