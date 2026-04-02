package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/netutil"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/util"
	"github.com/nunoOliveiraqwe/micro-proxy/middleware/country"
	"go.uber.org/zap"
)

func CountryBlockMiddleware(ctx context.Context, next http.HandlerFunc, middlewareConf Config) http.HandlerFunc {
	filter, err := initCountryFilter(middlewareConf)
	if err != nil {
		zap.S().Errorf("CountryBlockMiddleware: failed to initialize country filter: %v. Failing closed.", err)
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "CountryBlockMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		clientIP, err := netutil.GetClientIP(r)
		if err != nil {
			zap.S().Errorf("CountryBlockMiddleware: failed to get client IP: %v", err)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		addr, err := netip.ParseAddr(clientIP)
		if err != nil {
			zap.S().Errorf("CountryBlockMiddleware: failed to parse client IP %q: %v", clientIP, err)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		logger := GetRequestLoggerFromContext(r)
		if !filter.IsFromAllowedCountry(logger, r, addr) {
			zap.S().Infof("CountryBlockMiddleware: blocked request from IP %s", clientIP)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func initCountryFilter(middlewareConf Config) (*country.Filter, error) {
	cacheOpts, err := util.ParseCacheOptions(middlewareConf.Options)
	if err != nil {
		zap.S().Errorf("Failed to parse cache options: %v", err)
		return nil, err
	}

	// Parse source options
	sourceRaw, ok := middlewareConf.Options["source"]
	if !ok {
		return nil, fmt.Errorf("missing required 'source' option")
	}
	sourceMap, ok := sourceRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("'source' option must be a map")
	}

	modeRaw, ok := sourceMap["mode"]
	if !ok {
		return nil, fmt.Errorf("missing required 'source.mode' option")
	}
	mode, ok := modeRaw.(string)
	if !ok {
		return nil, fmt.Errorf("'source.mode' must be a string")
	}

	pathRaw, ok := sourceMap["path"]
	if !ok {
		return nil, fmt.Errorf("missing required 'source.path' option")
	}
	path, ok := pathRaw.(string)
	if !ok {
		return nil, fmt.Errorf("'source.path' must be a string")
	}

	var loader country.DbLoader
	var refreshInterval time.Duration
	switch strings.ToLower(mode) {
	case "local":
		loader = country.NewStaticFileDbLoader(path)
	case "remote":
		maxSizeStr := "300m" // default
		if ms, ok := sourceMap["max-size"]; ok {
			if s, ok := ms.(string); ok {
				maxSizeStr = s
			} else {
				return nil, fmt.Errorf("'source.max-size' must be a string")
			}
		}
		loader, err = country.NewDownloadDbLoader(path, maxSizeStr)
		if err != nil {
			return nil, fmt.Errorf("failed to create download db loader: %w", err)
		}

		// Parse optional refresh-interval (only applies to remote/download mode)
		if riRaw, ok := middlewareConf.Options["refresh-interval"]; ok {
			riStr, ok := riRaw.(string)
			if !ok {
				return nil, fmt.Errorf("'refresh-interval' must be a string (e.g. \"24h\", \"30m\")")
			}
			refreshInterval, err = util.ParseTimeString(riStr)
			if err != nil {
				return nil, fmt.Errorf("invalid 'refresh-interval' value %q: %w", riStr, err)
			}
			zap.S().Infof("Country DB refresh interval set to %s", refreshInterval)
		}
	default:
		return nil, fmt.Errorf("invalid 'source.mode' value %q, must be 'remote' or 'local'", mode)
	}

	// Parse list-mode
	listModeRaw, ok := middlewareConf.Options["list-mode"]
	if !ok {
		return nil, fmt.Errorf("missing required 'list-mode' option")
	}
	listModeStr, ok := listModeRaw.(string)
	if !ok {
		return nil, fmt.Errorf("'list-mode' must be a string")
	}

	var listMode country.ListMode
	switch strings.ToLower(listModeStr) {
	case "allow":
		listMode = country.AllowList
	case "block":
		listMode = country.BlockList
	default:
		return nil, fmt.Errorf("invalid 'list-mode' value %q, must be 'allow' or 'block'", listModeStr)
	}

	// Parse list (country codes)
	listRaw, ok := middlewareConf.Options["list"]
	if !ok {
		return nil, fmt.Errorf("missing required 'list' option")
	}
	listSlice, ok := listRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("'list' option must be an array of country code strings")
	}
	countryCodes := make([]string, 0, len(listSlice))
	for _, item := range listSlice {
		code, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("each entry in 'list' must be a string, got %T", item)
		}
		countryCodes = append(countryCodes, strings.ToUpper(code))
	}

	return country.NewFilter(cacheOpts, loader, listMode, countryCodes, refreshInterval)
}
