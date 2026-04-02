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
	MaxEntries      int
	TTL             time.Duration
	CleanupInterval time.Duration
}

type Cache[T EvictableEntry] struct {
	ctx             context.Context
	maxEntries      int
	ttl             time.Duration
	cleanupInterval time.Duration
	mu              sync.RWMutex
	cache           map[string]T
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
	return &CacheOptions{
		MaxEntries:      maxCacheSize,
		TTL:             ttl,
		CleanupInterval: cleanupInterval,
	}, nil
}

func NewCache[T EvictableEntry](options *CacheOptions) (*Cache[T], error) {
	c := &Cache[T]{
		ctx:             context.Background(),
		cleanupInterval: options.CleanupInterval,
		maxEntries:      options.MaxEntries,
		mu:              sync.RWMutex{},
		ttl:             options.TTL,
		cache:           make(map[string]T),
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
}

func (c *Cache[T]) GetValue(ip string) (T, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cache[ip]
	if !ok {
		var zero T
		return zero, ErrCacheMiss
	}
	if time.Since(entry.GetLastReadAt()) > c.ttl {
		var zero T
		return zero, ErrCacheMiss
	}
	entry.Touch()
	return entry, nil
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
