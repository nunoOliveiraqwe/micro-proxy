package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/middleware/ctx"
	"github.com/nunoOliveiraqwe/torii/middleware/ua"
	"go.uber.org/zap"
)

func UserAgentBlockMiddleware(context context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	cfg, err := parseUaConfig(context, conf)
	if err != nil {
		zap.S().Errorf("UserAgentBlockMiddleware: failed to parse configuration: %v. Failing closed.", err)
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "UserAgentBlockMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	uaBlocker, err := ua.NewUaBlocker(cfg)
	if err != nil {
		zap.S().Errorf("UserAgentBlockMiddleware: failed to parse configuration: %v. Failing closed.", err)
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "UserAgentBlockMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)

		clientIP, err := netutil.GetClientIP(r)
		if err != nil {
			logger.Warn("UserAgentBlockMiddleware: failed to get client IP:", zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		addr, err := netip.ParseAddr(clientIP)
		if err != nil {
			logger.Warn("BotDetectionMiddleware: failed to parse client IP", zap.String("clientIp", clientIP), zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if uaBlocker.IsIpInAllowList(addr) {
			logger.Debug("UserAgentBlockMiddleware: IP is in allow list, skipping UA check", zap.String("clientIp", clientIP))
			next(w, r)
			return
		}

		if uaBlocker.IsBlockedIP(addr.String()) {
			logger.Warn("UserAgentBlockMiddleware: blocked request from cached IP", zap.String("clientIp", clientIP))
			ctx.CreateAndAddBlockInfo(r, "ua-block", "cached blocked IP")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		userAgent := r.UserAgent()
		logger.Debug("UserAgentBlockMiddleware: checking user agent", zap.String("clientIp", clientIP), zap.String("user_agent", userAgent))

		if uaBlocker.IsBlockedUA(userAgent) {
			uaBlocker.CacheBlockedIP(addr.String())
			logger.Warn("UserAgentBlockMiddleware: blocked request", zap.String("clientIp", clientIP), zap.String("user_agent", userAgent))
			ctx.CreateAndAddBlockInfo(r, "ua-block", fmt.Sprintf("blocked user agent %s", userAgent))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func parseUaConfig(ctx context.Context, conf Config) (*ua.UaBlockerConfig, error) {
	if conf.Options == nil {
		return nil, fmt.Errorf("UserAgentBlockMiddleware: missing required options")
	}

	//i want to register this cache
	conf.Options[util.CacheInsightKey] = ctx.Value(ctxkeys.CacheInsightMgr)
	cacheOpts, err := util.ParseCacheOptions(conf.Options)
	if err != nil {
		return nil, err
	}

	if cacheOpts.IsUsingDefaultCacheName {
		cacheName, err2 := buildNameForConnection(ctx, "user-agent-block")
		if err2 != nil {
			zap.S().Warnf("UserAgentBlockMiddleware: failed to build connection name for cache options: %v. Using default cache name.", err2)
		} else {
			cacheOpts.CacheName = cacheName
		}
	}
	cacheOpts.TrackRate = true
	cacheOpts.Ctx = ctx

	zap.S().Debug("UserAgentBlockMiddleware: parsed cache options", zap.Any("cacheOpts", cacheOpts))

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

	lanAllowList, err := ParseStringSliceOpt(conf.Options, "lan-allow-list", []string{})
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
		LanAllowList:          lanAllowList,
	}, nil
}
