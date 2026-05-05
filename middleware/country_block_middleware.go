package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/middleware/country"
	"go.uber.org/zap"
)

func CountryBlockMiddleware(ctx context.Context, next http.HandlerFunc, middlewareConf Config) http.HandlerFunc {
	filter, err := initCountryFilter(ctx, middlewareConf)
	if err != nil {
		zap.S().Errorf("CountryBlockMiddleware: failed to initialize country filter: %v. Failing closed.", err)
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "CountryBlockMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		clientIP, err := netutil.GetClientIP(r)
		if err != nil {
			logger.Warn("CountryBlockMiddleware: failed to get client IP:", zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		addr, err := netip.ParseAddr(clientIP)
		if err != nil {
			logger.Warn("CountryBlockMiddleware: failed to parse client IP", zap.String("clientIp", clientIP), zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !filter.IsFromAllowedCountry(logger, r, addr) {
			logger.Warn("CountryBlockMiddleware: blocked request from IP", zap.String("clientIp", clientIP))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func initCountryFilter(ctx context.Context, middlewareConf Config) (*country.Filter, error) {
	//TODO -> this was to be one of the first middleware I wrote
	//and has such it doesn't use any of the helper functions for parsing options that I later wrote,
	//so it looks a bit more verbose than the other middleware in terms of option parsing.
	//I may want to refactor this at some point to be more consistent with the other middleware,
	//but for now it works and I don't want to risk breaking anything by changing it.

	middlewareConf.Options[util.CacheInsightKey] = ctx.Value(ctxkeys.CacheInsightMgr)
	cacheOpts, err := util.ParseCacheOptions(middlewareConf.Options)
	if err != nil {
		zap.S().Errorf("Failed to parse cache options: %v", err)
		return nil, err
	}

	if cacheOpts.IsUsingDefaultCacheName {
		cacheName, err2 := buildNameForConnection(ctx, "country-block")
		if err2 != nil {
			zap.S().Warnf("UserAgentBlockMiddleware: failed to build connection name for cache options: %v. Using default cache name.", err2)
		} else {
			cacheOpts.CacheName = cacheName
		}
	}
	cacheOpts.TrackRate = true
	cacheOpts.Ctx = ctx

	// Parse source options
	sourceRaw, ok := middlewareConf.Options["source"]
	if !ok {
		return nil, fmt.Errorf("missing required 'source' option")
	}
	sourceMap, ok := sourceRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("'source' option must be a map")
	}

	mode, err := ParseStringRequired(sourceMap, "mode")
	if err != nil {
		return nil, err
	}

	path, err := ParseStringRequired(sourceMap, "path")
	if err != nil {
		return nil, err
	}

	countryField, err := ParseStringRequired(sourceMap, "country-field")
	if err != nil {
		return nil, err
	}

	continentField, _ := ParseStringOpt(sourceMap, "continent-field", "")

	var loader country.DbLoader
	var refreshInterval time.Duration
	switch strings.ToLower(mode) {
	case "local":
		loader = country.NewStaticFileDbLoader(path)
	case "remote":
		maxSizeStr, err := ParseStringOpt(sourceMap, "max-size", "300m")
		if err != nil {
			return nil, err
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

	// Parse on-unknown (optional, defaults to block)
	onUnknown := false // default: block unknown
	ouStr, err := ParseStringOpt(middlewareConf.Options, "on-unknown", "block")
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(ouStr) {
	case "allow":
		onUnknown = true
	case "block":
		onUnknown = false
	default:
		return nil, fmt.Errorf("invalid 'on-unknown' value %q, must be 'allow' or 'block'", ouStr)
	}

	// Parse country-list-mode and country-list (optional, but at least one of country or continent must be set)
	var countryListMode country.ListMode
	var countryCodes []string
	if _, hasCountryList := middlewareConf.Options["country-list"]; hasCountryList {
		countryListMode, err = parseListMode(middlewareConf.Options, "country-list-mode")
		if err != nil {
			return nil, err
		}
		countryCodes, err = parseCodeList(middlewareConf.Options, "country-list")
		if err != nil {
			return nil, err
		}
	}

	// Parse continent-list-mode and continent-list (optional, requires continent-field in source)
	var continentListMode country.ListMode
	var continentCodes []string
	if _, hasContinentList := middlewareConf.Options["continent-list"]; hasContinentList {
		if continentField == "" {
			return nil, fmt.Errorf("'continent-list' requires 'source.continent-field' to be set")
		}
		continentListMode, err = parseListMode(middlewareConf.Options, "continent-list-mode")
		if err != nil {
			return nil, err
		}
		continentCodes, err = parseCodeList(middlewareConf.Options, "continent-list")
		if err != nil {
			return nil, err
		}
	}
	var lanAllowList []string
	if _, hasLanAllowList := middlewareConf.Options["lan-allow-list"]; hasLanAllowList {
		lanAllowList, err = parseCodeList(middlewareConf.Options, "lan-allow-list")
		if err != nil {
			return nil, err
		}
	}

	return country.NewFilter(ctx, cacheOpts, loader, countryListMode, countryCodes, continentListMode,
		continentCodes, refreshInterval, countryField, continentField, onUnknown, lanAllowList)
}

func parseListMode(options map[string]interface{}, key string) (country.ListMode, error) {
	str, err := ParseStringRequired(options, key)
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(str) {
	case "allow":
		return country.AllowList, nil
	case "block":
		return country.BlockList, nil
	default:
		return 0, fmt.Errorf("invalid '%s' value %q, must be 'allow' or 'block'", key, str)
	}
}

func parseCodeList(options map[string]interface{}, key string) ([]string, error) {
	raw, ok := options[key]
	if !ok {
		return nil, fmt.Errorf("missing required '%s' option", key)
	}
	slice, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("'%s' option must be an array of strings", key)
	}
	codes := make([]string, 0, len(slice))
	for _, item := range slice {
		code, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("each entry in '%s' must be a string, got %T", key, item)
		}
		codes = append(codes, strings.ToUpper(code))
	}
	return codes, nil
}
