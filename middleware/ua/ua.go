package ua

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/cloudflare/ahocorasick"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"go.uber.org/zap"
)

type uaCacheEntry struct {
	ip       string
	lastSeen time.Time
}

func (b *uaCacheEntry) Touch()                   { b.lastSeen = time.Now() }
func (b *uaCacheEntry) GetLastReadAt() time.Time { return b.lastSeen }

type UaBlockerConfig struct {
	BlockEmptyUA          bool
	DefaultListBlockedUAs []string
	ExtendedBlockedUAs    []string
	DefaultListAllowedUAs []string
	ExtendedAllowedUAs    []string
	LanAllowList          []string
	CacheOpt              *util.CacheOptions
}

type Blocker struct {
	blockEmptyUA bool
	blockedUas   *ahocorasick.Matcher       // Aho-Corasick matcher for blocked patterns; nil if none
	allowedUas   *ahocorasick.Matcher       // Aho-Corasick matcher for allow patterns; nil if none
	cache        *util.Cache[*uaCacheEntry] // nil if caching disabled
	lanAllowList *netutil.SubnetTrie
}

func NewUaBlocker(conf *UaBlockerConfig) (*Blocker, error) {
	hasPatterns := len(conf.DefaultListBlockedUAs) > 0 || len(conf.ExtendedBlockedUAs) > 0 ||
		len(conf.DefaultListAllowedUAs) > 0 || len(conf.ExtendedAllowedUAs) > 0
	if !hasPatterns && !conf.BlockEmptyUA {
		return nil, fmt.Errorf("UaBlockerConfig: no patterns configured and block-empty-ua is false — nothing to do")
	}

	cache, err := util.NewCache[*uaCacheEntry](conf.CacheOpt)
	if err != nil {
		return nil, fmt.Errorf("UaBlockerConfig: failed to create cache: %v", err)
	}
	blockedUas := make([]string, 0)
	allowedUas := make([]string, 0)

	if len(conf.DefaultListBlockedUAs) > 0 {
		defaultBlocked, err := getUasFromDefault(conf.DefaultListBlockedUAs)
		if err != nil {
			return nil, err
		}
		blockedUas = append(blockedUas, defaultBlocked...)
	}
	if len(conf.ExtendedBlockedUAs) > 0 {
		blockedUas = append(blockedUas, conf.ExtendedBlockedUAs...)
	}
	if len(conf.DefaultListAllowedUAs) > 0 {
		defaultAllowed, err := getUasFromDefault(conf.DefaultListAllowedUAs)
		if err != nil {
			return nil, err
		}
		allowedUas = append(allowedUas, defaultAllowed...)
	}
	if len(conf.ExtendedAllowedUAs) > 0 {
		allowedUas = append(allowedUas, conf.ExtendedAllowedUAs...)
	}

	var lanTrie *netutil.SubnetTrie
	if len(conf.LanAllowList) > 0 {
		zap.S().Infof("Building LAN allow list with %d entries", len(conf.LanAllowList))
		lanTrie, err = netutil.NewSubnetTrieFromStrings(conf.LanAllowList)
		if err != nil {
			return nil, fmt.Errorf("failed to build lan ip allow list: %w", err)
		}
	}

	return &Blocker{
		blockEmptyUA: conf.BlockEmptyUA,
		blockedUas:   buildAhoCorasick(blockedUas),
		allowedUas:   buildAhoCorasick(allowedUas),
		cache:        cache,
		lanAllowList: lanTrie,
	}, nil
}

func getUasFromDefault(defaults []string) ([]string, error) {
	uas := make([]string, 0)
	for _, category := range defaults {
		patterns, exists := DefaultBotPatterns[category]
		if !exists {
			zap.S().Error("UaBlockerConfig: unknown default category %q, skipping", category)
			return nil, fmt.Errorf("UaBlockerConfig: unknown default category %q", category)
		}
		uas = append(uas, patterns...)
	}
	return uas, nil
}

func buildAhoCorasick(patterns []string) *ahocorasick.Matcher {
	if len(patterns) == 0 {
		return nil
	}
	lowered := make([]string, len(patterns))
	for i, p := range patterns {
		lowered[i] = strings.ToLower(p)
	}
	m := ahocorasick.NewStringMatcher(lowered)
	return m
}

func (b *Blocker) IsBlockedIP(ip string) bool {
	_, err := b.cache.GetValue(ip)
	return err == nil
}

func (b *Blocker) IsIpInAllowList(ip netip.Addr) bool {
	if b.lanAllowList == nil {
		return false
	}
	return b.lanAllowList.Contains(ip)
}

func (b *Blocker) IsBlockedUA(ua string) bool {
	if ua == "" {
		return b.blockEmptyUA
	}

	lowered := []byte(strings.ToLower(ua))

	// Allow-list overrides, if the UA matches an allow pattern, it's never blocked
	if b.allowedUas != nil {
		if hits := b.allowedUas.MatchThreadSafe(lowered); len(hits) > 0 {
			return false
		}
	}

	if b.blockedUas != nil {
		if hits := b.blockedUas.MatchThreadSafe(lowered); len(hits) > 0 {
			return true
		}
	}

	return false
}

func (b *Blocker) CacheBlockedIP(ip string) {
	b.cache.CacheValue(ip, &uaCacheEntry{
		ip:       ip,
		lastSeen: time.Now(),
	})
}
