package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"

	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/middleware/honeypot"
	"go.uber.org/zap"
)

var honeypotDefaults = map[string][]string{
	"php": {
		"/.env", "/.env.local", "/.env.production", "/.env.backup",
		"/config.php", "/wp-config.php", "/wp-login.php", "/wp-admin",
		"/xmlrpc.php", "/shell.php", "/cmd.php", "/eval.php", "/webshell.php",
		"/phpmyadmin", "/pma", "/administrator",
	},
	"git": {
		"/.git/config", "/.git/HEAD", "/.svn/entries",
	},
	"infra": {
		"/actuator", "/actuator/env", "/actuator/health",
		"/metrics", "/.aws/credentials",
	},
	"backups": {
		"/backup.zip", "/backup.sql", "/db.sql", "/dump.sql", "/database.sql",
	},
	"cgi": {
		"/cgi-bin/",
	},
}

func HoneyPotMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	h, err := parseHoneyPotConfig(conf)
	if err != nil {
		zap.S().Errorf("HoneyPotMiddleware failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "HoneyPotMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	server, err := honeypot.NewHoneyPotServer(h)
	if err != nil {
		zap.S().Errorf("HoneyPotMiddleware failed to initialize configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "HoneyPotMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		clientIP, err := netutil.GetClientIP(r)
		if err != nil {
			logger.Warn("HoneyPotMiddleware: failed to get client IP:", zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		addr, err := netip.ParseAddr(clientIP)
		if err != nil {
			logger.Warn("HoneyPotMiddleware: failed to parse client IP", zap.String("clientIp", clientIP), zap.Error(err))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		isHoneyPotted := server.IsHoneyPottedIp(addr.String())

		logger.Debug("HoneyPotMiddleware: checking request against honeypot paths", zap.String("path", r.URL.Path), zap.Bool("is_honeypotted", isHoneyPotted))

		if isHoneyPotted {
			logger.Warn("HoneyPotMiddleware: blocked request from cached IP", zap.String("clientIp", clientIP))
			server.Serve(w, r, logger)
			return
		}

		logger.Debug("HoneyPotMiddleware: checking if request path is a honeypot path", zap.String("path", r.URL.Path), zap.Bool("is_honeypot_path", isHoneyPotted))
		isHoneyPath := server.IsHoneyPotPath(r.URL.Path)

		if isHoneyPath {
			logger.Warn("HoneyPotMiddleware: detected honeypot path access, caching IP",
				zap.String("clientIp", clientIP), zap.String("path", r.URL.Path))

			server.AddIpToHoneyPot(addr.String())
			server.Serve(w, r, logger)
			return
		}

		next(w, r)
	}
}

func parseHoneyPotConfig(conf Config) (*honeypot.HoneyPotConfig, error) {
	zap.S().Info("HoneyPotMiddleware: parsing configuration")
	if conf.Options == nil {
		return nil, fmt.Errorf("HoneyPotMiddleware: missing required options")
	}
	cacheOpts, err := util.ParseCacheOptions(conf.Options)
	if err != nil {
		return nil, err
	}

	var paths []string

	if raw, ok := conf.Options["defaults"]; ok {
		slice, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("HoneyPotMiddleware: 'defaults' option must be an array of strings")
		}
		for _, item := range slice {
			key, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("HoneyPotMiddleware: each entry in 'defaults' must be a string, got %T", item)
			}
			defaultPaths, exists := honeypotDefaults[key]
			if !exists {
				return nil, fmt.Errorf("HoneyPotMiddleware: unknown default group %q", key)
			}
			paths = append(paths, defaultPaths...)
		}
	}

	if raw, ok := conf.Options["paths"]; ok {
		slice, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("HoneyPotMiddleware: 'paths' option must be an array of strings")
		}
		for _, item := range slice {
			p, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("HoneyPotMiddleware: each entry in 'paths' must be a string, got %T", item)
			}
			paths = append(paths, p)
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("HoneyPotMiddleware: no paths configured; specify 'defaults' and/or 'paths'")
	}

	resp := honeypot.HoneyPotResponseConfig{
		StatusCode: http.StatusForbidden,
		Body:       "Forbidden",
	}
	if raw, ok := conf.Options["response"]; ok {
		respMap, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("HoneyPotMiddleware: 'response' option must be a map")
		}
		resp.TricksterMode = ParseBoolOpt(respMap, "trickster-mode", false)
		if !resp.TricksterMode {
			resp.StatusCode = ParseIntOpt(respMap, "status-code", http.StatusForbidden)
			body, err := ParseStringOpt(respMap, "body", "Forbidden")
			if err != nil {
				return nil, err
			}
			resp.Body = body
		} else {
			resp.MaxSlowTricks = ParseIntOpt(respMap, "max-slow-tricks", 10)
		}
	}

	return &honeypot.HoneyPotConfig{
		CacheOpts: cacheOpts,
		Paths:     paths,
		Response:  resp,
	}, nil
}
