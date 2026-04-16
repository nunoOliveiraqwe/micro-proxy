package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/nunoOliveiraqwe/torii/internal/proxyutil"
	"go.uber.org/zap"
)

type redirectOptions struct {
	mode       string
	statusCode int
	targetUrl  *url.URL
	dropPath   bool
	dropQuery  bool
}

type redirecter interface {
	redirect(w http.ResponseWriter, r *http.Request)
}

type internalRedirecter struct {
	opts  *redirectOptions
	proxy *httputil.ReverseProxy
}

type externalRedirecter struct {
	opts *redirectOptions
}

func (e *externalRedirecter) redirect(w http.ResponseWriter, r *http.Request) {
	redirectURL := e.buildRedirectURL(r)
	logger := GetRequestLoggerFromContext(r)
	logger.Info(fmt.Sprintf("Redirecting request from %s to %s", r.Host, redirectURL))
	http.Redirect(w, r, redirectURL, e.opts.statusCode)
}

func (e *externalRedirecter) buildRedirectURL(r *http.Request) string {
	u := *e.opts.targetUrl

	// If the target had no scheme, infer from the incoming request
	if u.Scheme == "" {
		if r.TLS != nil {
			u.Scheme = "https"
		} else {
			u.Scheme = "http"
		}
	}

	if !e.opts.dropPath {
		u.Path = strings.TrimSuffix(u.Path, "/") + r.URL.Path
	}
	if !e.opts.dropQuery {
		if u.RawQuery != "" && r.URL.RawQuery != "" {
			u.RawQuery = u.RawQuery + "&" + r.URL.RawQuery
		} else if r.URL.RawQuery != "" {
			u.RawQuery = r.URL.RawQuery
		}
	}

	return u.String()
}

type schemeOnlyRedirecter struct {
	opts *redirectOptions
}

func (e *schemeOnlyRedirecter) redirect(w http.ResponseWriter, r *http.Request) {
	redirectURL := e.buildRedirectURL(r)
	logger := GetRequestLoggerFromContext(r)
	logger.Info(fmt.Sprintf("Scheme-only redirecting request from %s to %s", r.Host, redirectURL))
	http.Redirect(w, r, redirectURL, e.opts.statusCode)
}

func (e *schemeOnlyRedirecter) buildRedirectURL(r *http.Request) string {
	u := url.URL{
		Scheme: e.opts.targetUrl.Scheme,
		Host:   r.Host,
	}
	if !e.opts.dropPath {
		u.Path = r.URL.Path
	}
	if !e.opts.dropQuery {
		u.RawQuery = r.URL.RawQuery
	}
	return u.String()
}

func newInternalRedirecter(opts *redirectOptions) (*internalRedirecter, error) {
	proxy, err := proxyutil.NewReverseProxy(opts.targetUrl.String(), proxyutil.ProxyOptions{
		DropPath:  opts.dropPath,
		DropQuery: opts.dropQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build internal redirect proxy: %w", err)
	}
	return &internalRedirecter{opts: opts, proxy: proxy}, nil
}

func (e *internalRedirecter) redirect(w http.ResponseWriter, r *http.Request) {
	logger := GetRequestLoggerFromContext(r)
	logger.Info(fmt.Sprintf("Internally redirecting request from %s to %s", r.Host, e.opts.targetUrl.String()))
	e.proxy.ServeHTTP(w, r)
}

func RedirectMiddleware(_ context.Context, _ http.HandlerFunc, conf Config) http.HandlerFunc {
	opts, err := parseRedirectConf(conf)
	if err != nil {
		zap.S().Errorf("RedirectMiddleware: failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "RedirectMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}
	var r redirecter
	if opts.mode == "external" {
		r = &externalRedirecter{opts: opts}
	} else if opts.mode == "external-scheme-only" {
		r = &schemeOnlyRedirecter{opts: opts}
	} else {
		ir, irErr := newInternalRedirecter(opts)
		if irErr != nil {
			zap.S().Errorf("RedirectMiddleware: failed to build internal redirecter: %v. Failing closed.", irErr)
			return func(writer http.ResponseWriter, request *http.Request) {
				http.Error(writer, "RedirectMiddleware misconfigured", http.StatusServiceUnavailable)
			}
		}
		r = ir
	}
	return func(writer http.ResponseWriter, request *http.Request) {
		r.redirect(writer, request)
	}
}

func parseRedirectConf(conf Config) (*redirectOptions, error) {
	zap.S().Debug("RedirectMiddleware: parsing configuration")
	if conf.Options == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}

	mode, err := ParseStringOpt(conf.Options, "mode", "external")
	if err != nil {
		return nil, err
	}
	if mode != "internal" && mode != "external" && mode != "external-scheme-only" {
		return nil, fmt.Errorf("invalid 'mode' option: must be 'internal', 'external', or 'external-scheme-only'")
	}

	var statusCode int
	if mode == "external" || mode == "external-scheme-only" {
		statusCodeRaw, ok := conf.Options["status-code"]
		if !ok {
			statusCode = 302
		} else {
			statusCode, err = strconv.Atoi(fmt.Sprintf("%v", statusCodeRaw))
			if err != nil || statusCode < 300 || statusCode > 399 {
				return nil, fmt.Errorf("invalid 'status-code' option: %v", statusCodeRaw)
			}
		}
	}

	target, err := ParseStringRequired(conf.Options, "target")
	if err != nil {
		return nil, err
	}

	zap.S().Debugf("RedirectMiddleware: successfully parsed configuration with mode %q, status code %d and target %q", mode, statusCode, target)

	// external-scheme-only requires target to be a plain scheme ("https" or "http").
	if mode == "external-scheme-only" {
		scheme := strings.ToLower(strings.TrimSuffix(target, "://"))
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("'target' for external-scheme-only mode must be 'http' or 'https'")
		}
		return &redirectOptions{
			mode:       mode,
			statusCode: statusCode,
			targetUrl:  &url.URL{Scheme: scheme},
			dropPath:   ParseBoolOpt(conf.Options, "drop-path", false),
			dropQuery:  ParseBoolOpt(conf.Options, "drop-query", false),
		}, nil
	}

	parsed, parseErr := url.Parse(target)

	// If url.Parse failed or didn't find a scheme+host, try treating the raw value as host:port
	// and construct a proper URL from it
	if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		zap.S().Debug("RedirectMiddleware: 'target' has no scheme, trying as host:port")
		host, port, splitErr := net.SplitHostPort(target)
		if splitErr != nil {
			return nil, fmt.Errorf("'target' option is not a valid URL or host:port: %v", splitErr)
		}
		// For internal mode, we need a full URL for the reverse proxy
		if mode == "internal" {

			parsed = &url.URL{
				Scheme: "http",
				Host:   net.JoinHostPort(host, port),
			}
		} else {
			// For external mode, keep it without scheme — buildRedirectURL will infer it
			parsed = &url.URL{
				Host: net.JoinHostPort(host, port),
			}
		}
	}

	return &redirectOptions{
		mode:       mode,
		statusCode: statusCode,
		targetUrl:  parsed,
		dropPath:   ParseBoolOpt(conf.Options, "drop-path", true),
		dropQuery:  ParseBoolOpt(conf.Options, "drop-query", true),
	}, nil
}
