package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/middleware/country"
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

	countryFieldRaw, ok := sourceMap["country-field"]
	if !ok {
		return nil, fmt.Errorf("missing required 'source.country-field' option")
	}
	countryField, ok := countryFieldRaw.(string)
	if !ok {
		return nil, fmt.Errorf("'source.country-field' must be a string")
	}

	var continentField string
	if cfRaw, ok := sourceMap["continent-field"]; ok {
		continentField, ok = cfRaw.(string)
		if !ok {
			return nil, fmt.Errorf("'source.continent-field' must be a string")
		}
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

	// Parse on-unknown (optional, defaults to block)
	onUnknown := false // default: block unknown
	if ouRaw, ok := middlewareConf.Options["on-unknown"]; ok {
		ouStr, ok := ouRaw.(string)
		if !ok {
			return nil, fmt.Errorf("'on-unknown' must be a string")
		}
		switch strings.ToLower(ouStr) {
		case "allow":
			onUnknown = true
		case "block":
			onUnknown = false
		default:
			return nil, fmt.Errorf("invalid 'on-unknown' value %q, must be 'allow' or 'block'", ouStr)
		}
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

	return country.NewFilter(cacheOpts, loader, countryListMode, countryCodes, continentListMode, continentCodes, refreshInterval, countryField, continentField, onUnknown)
}

func parseListMode(options map[string]interface{}, key string) (country.ListMode, error) {
	raw, ok := options[key]
	if !ok {
		return 0, fmt.Errorf("missing required '%s' option", key)
	}
	str, ok := raw.(string)
	if !ok {
		return 0, fmt.Errorf("'%s' must be a string", key)
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
