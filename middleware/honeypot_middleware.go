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
	"github.com/nunoOliveiraqwe/torii/middleware/honeypot"
	"go.uber.org/zap"
)

// honeypotDefaults contains paths that are unambiguously malicious — no
// legitimate client would ever request them.  Unlike Coraza (which scores
// each request independently), the honeypot **caches the source IP**, so a
// false positive here blocks ALL future requests from that address.
// Keep this list tight: high-confidence traps only.  Leave grey-area
// detection (scanners, anomalous headers, etc.) to the WAF.
//
// Path syntax:
//
//	"/foo"   — prefix match  (strings.HasPrefix)
//	"*/foo"  — contains match (strings.Contains) — catches /any/prefix/foo
var honeypotDefaults = map[string][]string{
	"php": {
		// Environment files — should never be served through a reverse proxy
		"/.env", "/.env.local", "/.env.production", "/.env.backup",
		"/.env.dev", "/.env.staging", "/.env.old", "/.env.save",
		// phpinfo — wildcard catches /public/phpinfo.php, /old/phpinfo.php,
		// /smtp/phpinfo.php, /cpanel/phpinfo.php, /mail/phpinfo.php, etc.
		"*/phpinfo.php", "*/phpinfo", "*/php-info.php",
		"/info.php", "/php.ini", "/test.php", "/debug.php",
		"/server-status.php", "/server-info.php",
		// phpinfo short-name aliases used by scanners
		"/p.php", "/pi.php", "/i.php", "/pinfo.php", "/php.php",
		// Symfony / Laravel profiler & environment leaks
		"/_profiler/phpinfo", "/_environment",
		"*/index.php/_environment",
		// WordPress probes — only useful if you're NOT proxying WordPress
		"/wp-config.php", "/wp-config.php.bak", "/wp-config.php.old",
		"/wp-login.php", "/wp-admin/install.php",
		"/wp-content/debug.log",
		"/xmlrpc.php",
		// Webshells — no legitimate reason for these to exist
		"/shell.php", "/cmd.php", "/eval.php", "/webshell.php",
		"/c99.php", "/r57.php", "/b374k.php", "/wso.php",
		// Database admin panels
		"/phpmyadmin", "/pma", "/myadmin", "/dbadmin",
	},
	"git": {
		"/.git/config", "/.git/HEAD", "/.git/index", "/.git/logs/HEAD",
		"/.svn/entries", "/.svn/wc.db",
		"/.hg/hgrc",
	},
	"infra": {
		// Cloud credential files — never web-accessible
		"/.aws/credentials", "/.aws/config",
		"/.docker/config.json",
		"/.kube/config",
		// Actuator endpoints that leak secrets or memory
		"/actuator/env", "/actuator/heapdump", "/actuator/jolokia",
		// .NET / IIS configuration — never served intentionally
		"/web.config", "/appsettings.json",
	},
	"secrets": {
		// SSH keys
		"/.ssh/id_rsa", "/.ssh/id_ed25519", "/.ssh/id_ecdsa", "/.ssh/authorized_keys",
		// Apache auth files
		"/.htpasswd", "/.htaccess",
		// Package manager credentials
		"/.npmrc", "/.pypirc", "/.gem/credentials",
		"/.composer/auth.json", "/.m2/settings.xml",
		// CI/CD tokens
		"/.travis.yml", "/.circleci/config.yml",
		// Private keys
		"/server.key", "/private.key", "/ssl/private.key",
		"/privatekey.pem", "/server.pem",
	},
	"iot": {
		// D-Link / router exploitation
		"/HNAP1/",
		// Router/CPE config endpoints
		"/goform/set_LimitClient_cfg", "/goform/webLogin",
		"/setup.cgi", "/cgi-bin/luci",
		// DVR/Camera panels
		"/dvr/cmd", "/webcam", "/onvif/device_service",
		"/录像机",
	},
	"backups": {
		"/backup.zip", "/backup.tar.gz", "/backup.sql", "/backup.sql.gz",
		"/db.sql", "/dump.sql", "/database.sql", "/data.sql",
		"/mysql.sql", "/db_backup.sql",
		"/site.tar.gz", "/www.zip", "/htdocs.zip",
		"/public.zip", "/html.zip",
	},
	"cgi": {
		"/cgi-bin/", "/cgi-bin/test-cgi",
		"/cgi-bin/printenv", "/cgi-bin/php",
		"/cgi-bin/bash", "/cgi-bin/sh",
	},
}

func HoneyPotMiddleware(context context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	h, err := parseHoneyPotConfig(context, conf)
	if err != nil {
		zap.S().Errorf("HoneyPotMiddleware failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "HoneyPotMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	honeyServer, err := honeypot.NewHoneyPotServer(h)
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

		isHoneyPotted := honeyServer.IsHoneyPottedIp(addr.String())

		logger.Debug("HoneyPotMiddleware: checking request against honeypot paths", zap.String("path", r.URL.Path), zap.Bool("is_honeypotted", isHoneyPotted))

		if isHoneyPotted {
			logger.Warn("HoneyPotMiddleware: blocked request from cached IP", zap.String("clientIp", clientIP))
			ctx.CreateAndAddBlockInfo(r, "honeypot", "cached honeypot IP")
			honeyServer.Serve(w, r, logger)
			return
		}

		logger.Debug("HoneyPotMiddleware: checking if request path is a honeypot path", zap.String("path", r.URL.Path))
		isHoneyPath := honeyServer.IsHoneyPotPath(r.URL.Path)

		if isHoneyPath {
			logger.Warn("HoneyPotMiddleware: detected honeypot path access, caching IP",
				zap.String("clientIp", clientIP), zap.String("path", r.URL.Path))

			honeyServer.AddIpToHoneyPot(addr.String())
			ctx.CreateAndAddBlockInfo(r, "honeypot", fmt.Sprintf("honeypot path: %s", r.URL.Path))
			honeyServer.Serve(w, r, logger)
			return
		}

		next(w, r)
	}
}

func parseHoneyPotConfig(ctx context.Context, conf Config) (*honeypot.HoneyPotConfig, error) {
	zap.S().Info("HoneyPotMiddleware: parsing configuration")
	if conf.Options == nil {
		return nil, fmt.Errorf("HoneyPotMiddleware: missing required options")
	}
	//i want to register this cache
	conf.Options[util.CacheInsightKey] = ctx.Value(ctxkeys.CacheInsightMgr)
	cacheOpts, err := util.ParseCacheOptions(conf.Options)
	if err != nil {
		return nil, err
	}

	cacheOpts.TrackRate = true
	cacheOpts.Ctx = ctx

	if cacheOpts.IsUsingDefaultCacheName {
		cacheName, err2 := buildNameForConnection(ctx, "honeypot")
		if err2 != nil {
			zap.S().Warnf("HoneyPotMiddleware: failed to build connection name for cache options: %v. Using default cache name.", err2)
		} else {
			cacheOpts.CacheName = cacheName
		}
	}

	zap.S().Debug("HoneyPotMiddleware: parsed cache options", zap.Any("cacheOpts", cacheOpts))

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
			contentType, err := ParseStringOpt(respMap, "content-type", "text/plain")
			if err != nil {
				return nil, err
			}
			resp.Body = body
			resp.ContentType = contentType
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
