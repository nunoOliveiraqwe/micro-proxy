package middleware

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

func StaticResponseMiddleware(_ context.Context, _ http.HandlerFunc, conf Config) http.HandlerFunc {
	statusCode, body, contentType, headers, err := parseStaticResponseConfig(conf)
	if err != nil {
		zap.S().Errorf("StaticResponseMiddleware failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "StaticResponseMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		logger.Debug("StaticResponseMiddleware: returning static response", zap.Int("statusCode", statusCode))
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		if body != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(statusCode)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}
}

func parseStaticResponseConfig(conf Config) (int, string, string, map[string]string, error) {
	zap.S().Debug("Parsing static response config")

	statusCode, err := ParseIntOptRequired(conf.Options, "status-code")
	if err != nil {
		return 0, "", "", nil, err
	}

	if statusCode < 100 || statusCode > 599 {
		return 0, "", "", nil, fmt.Errorf("status-code must be a valid HTTP status code (100-599)")
	}

	body, err := ParseStringOpt(conf.Options, "response-body", "")
	if err != nil {
		return 0, "", "", nil, err
	}

	contentType, err := ParseStringOpt(conf.Options, "content-type", "text/plain; charset=utf-8")
	if err != nil {
		return 0, "", "", nil, err
	}

	headers := make(map[string]string)
	if raw, ok := conf.Options["headers"]; ok {
		if m, ok := raw.(map[string]interface{}); ok {
			for k, v := range m {
				headers[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	return statusCode, body, contentType, headers, nil
}
