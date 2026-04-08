package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

type redirectOptions struct {
	statusCode int
	target     string
	dropPath   bool
	dropQuery  bool
}

func RedirectMiddleware(_ context.Context, _ http.HandlerFunc, conf Config) http.HandlerFunc {
	opts, err := parseRedirectConf(conf)
	if err != nil {
		zap.S().Errorf("RedirectMiddleware: failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "RedirectMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := GetRequestLoggerFromContext(request)
		redirectURL := opts.buildRedirectURL(request)
		logger.Info(fmt.Sprintf("Redirecting request from %s to %s", request.Host, redirectURL))
		http.Redirect(writer, request, redirectURL, opts.statusCode)
	}
}

func (opts *redirectOptions) buildRedirectURL(r *http.Request) string {
	parsed, err := url.Parse(opts.target)
	if err != nil {
		return opts.target
	}
	if !opts.dropPath {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/") + r.URL.Path
	}
	if !opts.dropQuery {
		if parsed.RawQuery != "" && r.URL.RawQuery != "" {
			parsed.RawQuery = parsed.RawQuery + "&" + r.URL.RawQuery
		} else if r.URL.RawQuery != "" {
			parsed.RawQuery = r.URL.RawQuery
		}
	}
	return parsed.String()
}

func parseRedirectConf(conf Config) (*redirectOptions, error) {
	zap.S().Debug("RedirectMiddleware: parsing configuration")
	if conf.Options == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}

	statusCodeRaw, ok := conf.Options["status-code"]
	if !ok {
		return nil, fmt.Errorf("missing required option 'status-code'")
	}
	statusCode, err := strconv.Atoi(fmt.Sprintf("%v", statusCodeRaw))
	if err != nil || statusCode < 300 || statusCode > 399 {
		return nil, fmt.Errorf("invalid 'status-code' option: %v", statusCodeRaw)
	}

	target, err := ParseStringRequired(conf.Options, "target")
	if err != nil {
		return nil, err
	}

	zap.S().Debugf("RedirectMiddleware: successfully parsed configuration with status code %d and target %q", statusCode, target)

	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme == "" && parsed.Host == "") {
		zap.S().Debug("RedirectMiddleware: 'target' option is not a valid URL. Trying as hostPort")
		_, _, err = net.SplitHostPort(target)
		if err != nil {
			return nil, fmt.Errorf("'target' option is not a valid URL or host:port: %v", err)
		}
	}

	return &redirectOptions{
		statusCode: statusCode,
		target:     target,
		dropPath:   ParseBoolOpt(conf.Options, "drop-path", true),
		dropQuery:  ParseBoolOpt(conf.Options, "drop-query", true),
	}, nil
}
