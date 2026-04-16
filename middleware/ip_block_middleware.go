package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/middleware/ip_block"
	"go.uber.org/zap"
)

func IpBlockMiddleware(ctx context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	filter, err := initIpFilter(conf)
	if err != nil {
		zap.S().Errorf("IpBlockMiddleware: failed to initialize: %v. Failing closed.", err)
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "IpBlockMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		clientIP, err := netutil.GetClientIP(r)
		if err != nil {
			logger.Warn("IpBlockMiddleware: failed to get client IP", zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		blocked, err := filter.IsBlocked(clientIP)
		if err != nil {
			logger.Error("IpBlockMiddleware: error checking IP", zap.String("clientIp", clientIP), zap.Error(err))
			metrics.CreateAndAddBlockInfo(r, "IpBlock", fmt.Sprintf("An error occurred check if IP %s is blocked. err = %s",
				clientIP,
				err.Error()))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if blocked {
			logger.Warn("IpBlockMiddleware: blocked request", zap.String("clientIp", clientIP))
			metrics.CreateAndAddBlockInfo(r, "IpBlock", fmt.Sprintf("Blocked IP: %s", clientIP))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func initIpFilter(conf Config) (*ip_block.IpFilter, error) {
	opts := conf.Options

	allowList, err := ParseStringSliceOpt(opts, "allow", nil)
	if err != nil {
		return nil, err
	}

	blockList, err := ParseStringSliceOpt(opts, "block", nil)
	if err != nil {
		return nil, err
	}

	// If AbuseIPDB is configured, use it as the block list source
	// (with the static allow list as a whitelist override)
	if apiKey, _ := ParseStringOpt(opts, "abuseipdb-api-key", ""); apiKey != "" {
		confidence := ParseIntOpt(opts, "abuseipdb-confidence-minimum", 90)
		refreshInterval, err := ParseStringRequired(opts, "abuseipdb-refresh-interval")
		if err != nil {
			return nil, err
		}

		loader, err := ip_block.NewAbuseIpDbBlockListLoader(apiKey, confidence, refreshInterval, allowList)
		if err != nil {
			return nil, err
		}
		return ip_block.NewIpFilter(loader)
	}

	loader := ip_block.NewStaticLoader(allowList, blockList)
	return ip_block.NewIpFilter(loader)
}
