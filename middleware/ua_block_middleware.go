package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"

	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/middleware/ua"
	"go.uber.org/zap"
)

func UserAgentBlockMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	cfg, err := parseUaConfig(conf)
	if err != nil {
		zap.S().Errorf("BotDetectionMiddleware: failed to parse configuration: %v. Failing closed.", err)
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "BotDetectionMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	uaBlocker, err := ua.NewUaBlocker(cfg)
	if err != nil {
		zap.S().Errorf("BotDetectionMiddleware: failed to parse configuration: %v. Failing closed.", err)
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "BotDetectionMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)

		clientIP, err := netutil.GetClientIP(r)
		if err != nil {
			logger.Warn("BotDetectionMiddleware: failed to get client IP:", zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		addr, err := netip.ParseAddr(clientIP)
		if err != nil {
			logger.Warn("BotDetectionMiddleware: failed to parse client IP", zap.String("clientIp", clientIP), zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if uaBlocker.IsBlockedIP(addr.String()) {
			logger.Warn("BotDetectionMiddleware: blocked request from cached IP", zap.String("clientIp", clientIP))
			metrics.CreateAndAddBlockInfo(r, "ua-block", "cached blocked IP")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		userAgent := r.UserAgent()
		logger.Debug("BotDetectionMiddleware: checking user agent", zap.String("clientIp", clientIP), zap.String("user_agent", userAgent))

		if uaBlocker.IsBlockedUA(userAgent) {
			uaBlocker.CacheBlockedIP(addr.String())
			logger.Warn("BotDetectionMiddleware: blocked request", zap.String("clientIp", clientIP), zap.String("user_agent", userAgent))
			metrics.CreateAndAddBlockInfo(r, "ua-block", fmt.Sprintf("blocked user agent %s", userAgent))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func parseUaConfig(conf Config) (*ua.UaBlockerConfig, error) {
	if conf.Options == nil {
		return nil, fmt.Errorf("BotDetectionMiddleware: missing required options")
	}

	cacheOpts, err := util.ParseCacheOptions(conf.Options)
	if err != nil {
		return nil, err
	}

	blockEmptyUA := ParseBoolOpt(conf.Options, "block-empty-ua", true)

	defaultBlocked, err := ParseStringSliceOpt(conf.Options, "block-defaults", []string{})
	if err != nil {
		return nil, err
	}

	extraBlocked, err := ParseStringSliceOpt(conf.Options, "block", []string{})
	if err != nil {
		return nil, err
	}

	defaultAllow, err := ParseStringSliceOpt(conf.Options, "allow-defaults", []string{})
	if err != nil {
		return nil, err
	}

	extraAllow, err := ParseStringSliceOpt(conf.Options, "allow", []string{})
	if err != nil {
		return nil, err
	}

	return &ua.UaBlockerConfig{
		BlockEmptyUA:          blockEmptyUA,
		DefaultListBlockedUAs: defaultBlocked,
		DefaultListAllowedUAs: defaultAllow,
		ExtendedBlockedUAs:    extraBlocked,
		ExtendedAllowedUAs:    extraAllow,
		CacheOpt:              cacheOpts,
	}, nil
}
