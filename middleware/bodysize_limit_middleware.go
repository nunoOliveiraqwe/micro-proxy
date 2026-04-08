package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/util"
	"go.uber.org/zap"
)

func BodySizeLimitMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	maxSize, err := parseBodySizeLimitConfig(conf)
	if err != nil {
		zap.S().Errorf("BodySizeLimitMiddleware failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "BodySizeLimitMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := GetRequestLoggerFromContext(request)
		logger.Info("Applying body size limit middleware for request", zap.Int64("max_size", maxSize))
		request.Body = http.MaxBytesReader(writer, request.Body, maxSize)
		next(writer, request)
	}
}

func parseBodySizeLimitConfig(conf Config) (int64, error) {
	zap.S().Debug("Parsing body size limit config")
	limit, err := ParseStringRequired(conf.Options, "body_size_limit")
	if err != nil {
		return 0, err
	}
	size, err := util.ParseSizeString(limit)
	if err != nil {
		return 0, err
	}
	if size <= 0 {
		return 0, fmt.Errorf("body_size_limit must be positive, got %d", size)
	}
	return size, nil
}
