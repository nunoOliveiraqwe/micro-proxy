package country

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/oschwald/maxminddb-golang/v2"
	"go.uber.org/zap"
)

// ListMode determines whether the country list acts as an allow list or a block list.
type ListMode int

const (
	// AllowList only permits traffic from countries in the list.
	AllowList ListMode = iota
	// BlockList only blocks traffic from countries in the list.
	BlockList
)

type countryRecord struct {
	Country struct {
		IsoCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

type clientEntry struct {
	logger      *zap.Logger
	ip          string
	IsAllowed   bool
	lastSeen    time.Time
	countryCode string
}

func (c *clientEntry) Touch() {
	c.logger.Info("Touching client entry for ", zap.String("IP", c.ip))
	c.lastSeen = time.Now()
}

func (c *clientEntry) GetLastReadAt() time.Time {
	return c.lastSeen
}

type Filter struct {
	mu              sync.RWMutex
	db              *maxminddb.Reader
	loader          DbLoader
	refreshInterval time.Duration
	clientCache     *util.Cache[*clientEntry]
	mode            ListMode
	countries       map[string]byte
}

func NewFilter(cacheOpts *util.CacheOptions, loader DbLoader, mode ListMode, countryCodes []string, refreshInterval time.Duration) (*Filter, error) {
	if mode != AllowList && mode != BlockList {
		return nil, fmt.Errorf("invalid list mode: %d, must be AllowList (%d) or BlockList (%d)", mode, AllowList, BlockList)
	}
	if len(countryCodes) == 0 {
		return nil, fmt.Errorf("country code list must not be empty")
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

	zap.S().Infof("Country filter configured in %s mode with %d countries", modeString(mode), len(countrySet))
	f := &Filter{
		clientCache:     cache,
		db:              reader,
		loader:          loader,
		refreshInterval: refreshInterval,
		mode:            mode,
		countries:       countrySet,
	}

	if refreshInterval > 0 && loader.isRefreshable() {
		f.startRefresh()
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

func (c *Filter) startRefresh() {
	zap.S().Infof("Starting country DB refresh goroutine with interval %s", c.refreshInterval)
	ticker := time.NewTicker(c.refreshInterval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			c.reloadDB()
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
		entry = c.lookupIPAndCacheValue(logger, ip)
	}
	if entry == nil {
		return false
	}
	if r != nil {
		ctx := context.WithValue(r.Context(), "country-code", entry.countryCode)
		*r = *r.WithContext(ctx)
	}
	return entry.IsAllowed
}

func (c *Filter) lookupIPAndCacheValue(logger *zap.Logger, ip netip.Addr) *clientEntry {
	isAllowed := false

	c.mu.RLock()
	result := c.db.Lookup(ip)
	c.mu.RUnlock()

	if result.Found() {
		var record countryRecord
		err := result.Decode(&record)
		if err != nil {
			logger.Error("Failed to decode country db result for IP ", zap.String("IP", ip.String()), zap.Error(err))
		} else {
			logger.Info("Decoded country db result for IP ", zap.String("IP", ip.String()), zap.String("Country", record.Country.IsoCode))
			isAllowed = c.countryIsInAllowList(record.Country.IsoCode)
			logger.Info("IP ", zap.String("IP", ip.String()), zap.Bool("IsAllowedCountry", isAllowed), zap.String("Country", record.Country.IsoCode))
		}

		entry := &clientEntry{
			logger:      logger,
			ip:          ip.String(),
			IsAllowed:   isAllowed,
			lastSeen:    time.Now(),
			countryCode: record.Country.IsoCode,
		}
		c.clientCache.CacheValue(ip.String(), entry)
		return entry
	}
	return nil
}

func (c *Filter) countryIsInAllowList(countryIsoCode string) bool {
	_, found := c.countries[countryIsoCode]
	switch c.mode {
	case AllowList:
		return found
	case BlockList:
		return !found
	default:
		return false
	}
}
