package util

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	defaultCacheTTL        = "1h"
	defaultCleanupInterval = "1h"
	defaultMaxEntries      = 100000
)

var ErrCacheMiss = fmt.Errorf("cache miss")

type EvictableEntry interface {
	Touch()
	GetLastReadAt() time.Time
}

type CacheOptions struct {
	manager                 *CacheInsightManager //if defined cache is registered with manager
	IsUsingDefaultCacheName bool
	TrackRate               bool
	CacheName               string
	MaxEntries              int
	TTL                     time.Duration
	CleanupInterval         time.Duration
}

type Cache[T EvictableEntry] struct {
	cacheName       string
	ctx             context.Context
	maxEntries      int
	ttl             time.Duration
	cleanupInterval time.Duration
	mu              sync.RWMutex
	cache           map[string]T
	tracker         Tracker
}

func ParseCacheOptions(configMap map[string]interface{}) (*CacheOptions, error) {
	cacheTtlStr := defaultCacheTTL
	cacheTtl, ok := configMap["cache-ttl"]
	if !ok {
		zap.S().Warnf("'cache-ttl' option not specified, defaulting to %s", cacheTtlStr)
	} else if str, ok := cacheTtl.(string); ok {
		cacheTtlStr = str
	}
	ttl, err := ParseTimeString(cacheTtlStr)
	if err != nil {
		zap.S().Errorf("Failed to parse TTL string %q: %v. Cannot initialize cache", cacheTtlStr, err)
		return nil, fmt.Errorf("invalid TTL string: %w", err)
	}

	cleanUpIntervalStr := defaultCleanupInterval
	cleanUpInterval, ok := configMap["cleanup-interval"]
	if !ok {
		zap.S().Warnf("'cleanup-interval' option not specified, defaulting to %s", cleanUpIntervalStr)
	} else if cleanUp, ok := cleanUpInterval.(string); ok {
		cleanUpIntervalStr = cleanUp
	}

	cleanupInterval, err2 := ParseTimeString(cleanUpIntervalStr)
	if err2 != nil {
		zap.S().Errorf("Failed to parse cleanup interval string %q: %v. Cannot initialize cache", cleanUpIntervalStr, err2)
		return nil, fmt.Errorf("invalid cleanup interval string: %w", err2)
	}

	maxCacheSize := defaultMaxEntries
	maxCache, ok := configMap["max-cache-size"]
	if !ok {
		zap.S().Warnf("RateLimitMiddleware: 'max-cache-size' option not specified, defaulting to %d", maxCacheSize)
	} else if m, ok := maxCache.(int); ok {
		maxCacheSize = m
	}

	isUsingDefaultCacheName := true
	cacheNameStr := fmt.Sprintf("cache-%d", time.Now().UnixNano())
	cacheName, ok := configMap["cache-name"]
	if ok {
		if m, ok := cacheName.(string); ok {
			cacheTtlStr = m
			isUsingDefaultCacheName = false
		}
	}
	trackRateBool := false
	trackRate, ok := configMap["track-rate"]
	if !ok {
		zap.S().Warnf("'track-rate' option not specified, defaulting to false")
	} else if m, ok := trackRate.(bool); ok {
		trackRateBool = m
	}

	var mgrManagerCasted *CacheInsightManager
	mgr, ok := configMap[CacheInsightKey]
	if ok {
		if m, ok := mgr.(*CacheInsightManager); ok {
			mgrManagerCasted = m
		}
	}

	return &CacheOptions{
		manager:                 mgrManagerCasted,
		IsUsingDefaultCacheName: isUsingDefaultCacheName,
		TrackRate:               trackRateBool,
		CacheName:               cacheNameStr,
		MaxEntries:              maxCacheSize,
		TTL:                     ttl,
		CleanupInterval:         cleanupInterval,
	}, nil
}

func NewCache[T EvictableEntry](options *CacheOptions) (*Cache[T], error) {
	rateTracker := NewNopRateTracker()
	if options.TrackRate {
		rateTracker = NewRateTracker()
	}
	c := &Cache[T]{
		cacheName:       options.CacheName,
		ctx:             context.Background(),
		cleanupInterval: options.CleanupInterval,
		maxEntries:      options.MaxEntries,
		mu:              sync.RWMutex{},
		ttl:             options.TTL,
		cache:           make(map[string]T),
		tracker:         rateTracker,
	}
	if options.manager != nil {
		options.manager.RegisterCache(c)
	}
	c.startCleanup()
	return c, nil
}

func (c *Cache[T]) CacheValue(ip string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) >= c.maxEntries {
		c.evictOldest()
	}
	value.Touch()
	c.cache[ip] = value
	c.tracker.Mark(1)
}

func (c *Cache[T]) GetValue(ip string) (T, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cache[ip]
	if !ok {
		c.tracker.MarkMiss()
		var zero T
		return zero, ErrCacheMiss
	}
	if time.Since(entry.GetLastReadAt()) > c.ttl {
		c.tracker.MarkMiss()
		var zero T
		return zero, ErrCacheMiss
	}
	c.tracker.MarkHit()
	entry.Touch()
	return entry, nil
}

func (c *Cache[T]) Evict(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

// only supposed to be called after a lock is held
func (c *Cache[T]) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.cache {
		if first || entry.GetLastReadAt().Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.GetLastReadAt()
			first = false
		}
	}

	if !first {
		delete(c.cache, oldestKey)
	}
}

func (c *Cache[T]) startCleanup() {
	ticker := time.NewTicker(c.cleanupInterval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			select {
			case <-c.ctx.Done():
				zap.S().Info("Cache cleanup goroutine exiting due to context cancellation")
				return
			default:
				// continue with cleanup
				c.sweep()
			}
		}
	}()
}

func (c *Cache[T]) sweep() {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-c.ttl)
	for ip, entry := range c.cache {
		if entry.GetLastReadAt().Before(cutoff) {
			delete(c.cache, ip)
		}
	}
}

func (c *Cache[T]) CacheName() string {
	return c.cacheName
}

func (c *Cache[T]) GetMaxLen() int {
	return c.maxEntries
}

func (c *Cache[T]) GetCurrentNumberOfElements() int {
	return len(c.cache)
}

func (c *Cache[T]) GetAllKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.cache))
	for k := range c.cache {
		keys = append(keys, k)
	}
	return keys
}

func (c *Cache[T]) CacheInsertionM1Rate() float64 {
	return c.tracker.M1Rate()
}

func (c *Cache[T]) CacheInsertionM5Rate() float64 {
	return c.tracker.M5Rate()
}

func (c *Cache[T]) CacheInsertionM15Rate() float64 {
	return c.tracker.M15Rate()
}

func (c *Cache[T]) CacheInsertionTotal() int64 {
	return c.tracker.Total()
}

func (c *Cache[T]) CacheHits() int64 {
	return c.tracker.Hits()
}

func (c *Cache[T]) CacheMisses() int64 {
	return c.tracker.Misses()
}
