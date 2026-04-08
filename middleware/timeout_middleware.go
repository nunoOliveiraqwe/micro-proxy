package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
	"go.uber.org/zap"
)

func TimeoutMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	timeout, err := parseTimeoutConfig(conf)
	if err != nil {
		zap.S().Errorf("TimeoutMiddleware failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "TimeoutMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}
	h := http.TimeoutHandler(next, timeout, "request timed out")
	return h.ServeHTTP
}

func parseTimeoutConfig(conf Config) (time.Duration, error) {
	zap.S().Debug("Parsing timeout config")
	timeoutStr, err := ParseStringRequired(conf.Options, "timeout_seconds")
	if err != nil {
		return 0, err
	}
	return util.ParseTimeString(timeoutStr)
}
