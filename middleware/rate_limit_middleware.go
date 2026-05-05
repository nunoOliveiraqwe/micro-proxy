package middleware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/middleware/ctx"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type rateLimitConf struct {
	Burst         int
	RatePerSecond float64
	Mode          string
	CacheOpt      *util.CacheOptions
}

type limiter interface {
	limit(r *http.Request, w http.ResponseWriter) bool
}

type globalLimiter struct {
	internalLimiter *rate.Limiter
	retryAfter      string
}

type clientEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type perClientLimiter struct {
	clientCache   *util.Cache[*clientEntry]
	ratePerSecond float64
	burst         int
	retryAfter    string
}

func (e *clientEntry) Touch() {
	e.lastSeen = time.Now()
}

func (e *clientEntry) GetLastReadAt() time.Time {
	return e.lastSeen
}

func (g *globalLimiter) limit(r *http.Request, w http.ResponseWriter) bool {
	if g.internalLimiter.Allow() {
		return true
	}
	w.Header().Set("Retry-After", g.retryAfter)
	ctx.CreateAndAddBlockInfo(r, "rate-limit", fmt.Sprintf("global rate limit of %f req/s exceeded", g.internalLimiter.Limit()))
	http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
	return false
}

func (l *perClientLimiter) limit(r *http.Request, w http.ResponseWriter) bool {
	logger := GetRequestLoggerFromContext(r)
	ipAddr, err := netutil.GetClientIP(r)
	if err != nil {
		logger.Warn("RateLimitMiddleware: failed to extract client IP", zap.Error(err))
		ctx.CreateAndAddBlockInfo(r, "rate-limit", "failed to extract client IP")
		w.Header().Set("Retry-After", l.retryAfter)
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		return false
	}
	entry, err := l.clientCache.GetValue(ipAddr)
	if err != nil && errors.Is(err, util.ErrCacheMiss) {
		entry = &clientEntry{
			limiter: rate.NewLimiter(rate.Limit(l.ratePerSecond), l.burst),
		}
		l.clientCache.CacheValue(ipAddr, entry)
	} else if err != nil {
		logger.Warn("RateLimitMiddleware: failed to get client entry from cache for IP", zap.String("Ip", ipAddr), zap.Error(err))
		ctx.CreateAndAddBlockInfo(r, "rate-limit", "cache error on get")
		w.Header().Set("Retry-After", l.retryAfter)
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		return false
	}
	if entry.limiter.Allow() {
		return true
	}
	logger.Warn("RateLimitMiddleware: rate limit exceeded for client IP", zap.String("Ip", ipAddr))
	ctx.CreateAndAddBlockInfo(r, "rate-limit", fmt.Sprintf("rate limit of %f req/s exceeded", entry.limiter.Limit()))
	w.Header().Set("Retry-After", l.retryAfter)
	http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
	return false
}

func RateLimitMiddleware(ctx context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	limitConf, err := parseRateLimitConfig(ctx, conf)
	if err != nil {
		zap.S().Errorf("Failed to parse rate limit middleware configuration: %v. Failing closed.", err)
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "rate limiter misconfigured", http.StatusServiceUnavailable)
		}
	}
	l, err := newLimiter(limitConf)
	if err != nil {
		zap.S().Errorf("Failed to initialize rate limiter: %v. Failing closed.", err)
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "rate limiter misconfigured", http.StatusServiceUnavailable)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if l.limit(r, w) {
			next.ServeHTTP(w, r)
		}
	}
}

func computeRetryAfter(ratePerSecond float64) string {
	if ratePerSecond <= 0 {
		return "1"
	}
	secs := int(math.Ceil(1.0 / ratePerSecond))
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}

func newLimiter(c *rateLimitConf) (limiter, error) {
	retryAfter := computeRetryAfter(c.RatePerSecond)
	if strings.EqualFold(c.Mode, "per-client") {
		cache, err := util.NewCache[*clientEntry](c.CacheOpt)
		if err != nil {
			zap.S().Errorf("Failed to initialize client cache for per-client rate limiter: %v. Failing closed.", err)
			return nil, err
		}
		pcl := &perClientLimiter{
			clientCache:   cache,
			ratePerSecond: c.RatePerSecond,
			burst:         c.Burst,
			retryAfter:    retryAfter,
		}
		return pcl, nil
	}
	return &globalLimiter{
		internalLimiter: rate.NewLimiter(rate.Limit(c.RatePerSecond), c.Burst),
		retryAfter:      retryAfter,
	}, nil
}

func parseRateLimitConfig(ctx context.Context, conf Config) (*rateLimitConf, error) {
	zap.S().Debug("Parsing rate limit middleware configuration")
	reqLimit, ok := conf.Options["limiter-req"]
	if !ok {
		zap.S().Error("RateLimitMiddleware: missing required option 'limiter-req'")
		return nil, fmt.Errorf("missing required option 'limiter-req'")
	}
	reqLimitMap, ok := reqLimit.(map[string]interface{})
	if !ok {
		zap.S().Error("RateLimitMiddleware: 'limiter-req' option must be a map[string]interface{}")
		return nil, fmt.Errorf("'limiter-req' option must be a map[string]interface{}")
	}

	burstInt, err := ParseIntOptRequired(reqLimitMap, "burst")
	if err != nil {
		return nil, err
	}
	if burstInt <= 0 {
		return nil, fmt.Errorf("'burst' must be a positive integer, got %d", burstInt)
	}

	ratePsF, err := ParseFloatOptRequired(reqLimitMap, "rate-per-second")
	if err != nil {
		return nil, err
	}
	if ratePsF <= 0 {
		return nil, fmt.Errorf("'rate-per-second' must be a positive number, got %f", ratePsF)
	}

	modeStr, err := ParseStringOpt(conf.Options, "mode", "global")
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(modeStr, "global") && !strings.EqualFold(modeStr, "per-client") {
		zap.S().Errorf("RateLimitMiddleware: invalid 'mode' option value '%s', must be 'global' or 'per-client'", modeStr)
		return nil, fmt.Errorf("invalid 'mode' option value '%s', must be 'global' or 'per-client'", modeStr)
	}

	var cacheOpts *util.CacheOptions
	if strings.EqualFold(modeStr, "per-client") {
		cacheOpts, err = util.ParseCacheOptions(conf.Options)
		if err != nil {
			zap.S().Errorf("Failed to parse cache options for per-client rate limiter: %v. Failing closed.", err)
			return nil, fmt.Errorf("failed to parse cache options: %w", err)
		}
		cacheOpts.Ctx = ctx
	}
	return &rateLimitConf{
		CacheOpt:      cacheOpts,
		Burst:         burstInt,
		RatePerSecond: ratePsF,
		Mode:          modeStr,
	}, nil
}
