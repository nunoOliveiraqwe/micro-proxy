package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
	"go.uber.org/zap"
)

type circuitBreakerOpts struct {
	failureThreshold  int32
	recoveryDuration  time.Duration
	halfOpenThreshold int32
}

type targetMonitor struct {
	currentFailureCount atomic.Int32
	lastFailureNano     atomic.Int64
	failureThreshold    int32
	recoveryDuration    time.Duration
	halfOpenThreshold   int32
}

type circuitBreakerResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (cbw *circuitBreakerResponseWriter) WriteHeader(statusCode int) {
	cbw.statusCode = statusCode
	cbw.ResponseWriter.WriteHeader(statusCode)
}

func CircuitBreakerMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	opts, err := parseCircuitBreakerConfig(conf)
	if err != nil {
		zap.S().Errorf("CircuitBreakerMiddleware failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "CircuitBreakerMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}
	m := targetMonitor{
		failureThreshold:  opts.failureThreshold,
		recoveryDuration:  opts.recoveryDuration,
		halfOpenThreshold: opts.halfOpenThreshold,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		logger.Info("Applying circuit breaker for request")

		count := m.currentFailureCount.Load()
		if count >= m.failureThreshold {
			lastFail := time.Unix(0, m.lastFailureNano.Load())
			timeSinceLastFailure := time.Since(lastFail)
			if timeSinceLastFailure < m.recoveryDuration {
				logger.Info("Circuit breaker is open. Rejecting request.", zap.Duration("timeSinceLastFailure", timeSinceLastFailure))
				http.Error(w, "Service unavailable due to high error rate", http.StatusServiceUnavailable)
				return
			}
			logger.Info("Recovery duration has passed since last failure. Allowing request to test if service has recovered.")
			newCount := count - m.halfOpenThreshold
			if newCount < 0 {
				newCount = 0
			}
			m.currentFailureCount.CompareAndSwap(count, newCount)
		}

		responseWriter := &circuitBreakerResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(responseWriter, r)

		isFailure := false
		if responseWriter.statusCode >= 500 {
			isFailure = true
			logger.Info("Request resulted in server error", zap.Int("statusCode", responseWriter.statusCode))
		} else if err := r.Context().Err(); errors.Is(err, context.DeadlineExceeded) {
			isFailure = true
			logger.Info("Request resulted in timeout")
		}

		if isFailure {
			newCount := m.currentFailureCount.Add(1)
			m.lastFailureNano.Store(time.Now().UnixNano())
			logger.Info("Incremented circuit breaker failure count", zap.Int32("failureCount", newCount))
		} else {
			for {
				cur := m.currentFailureCount.Load()
				if cur <= 0 {
					break
				}
				if m.currentFailureCount.CompareAndSwap(cur, cur-1) {
					break
				}
			}
		}
	}
}

func parseCircuitBreakerConfig(conf Config) (*circuitBreakerOpts, error) {
	zap.S().Debug("Parsing circuit breaker config")
	if conf.Options == nil {
		return nil, fmt.Errorf("options cannot be nil")
	}
	failureThreshold, err := ParseIntOptRequired(conf.Options, "failure_threshold")
	if err != nil {
		return nil, err
	}
	recTimeStr, err := ParseStringRequired(conf.Options, "recovery_time")
	if err != nil {
		return nil, err
	}
	recoveryTime, err := util.ParseTimeString(recTimeStr)
	if err != nil {
		return nil, err
	}

	halfOpenThreshold := ParseIntOpt(conf.Options, "half_open_success_threshold", 3)
	if halfOpenThreshold < 1 {
		return nil, fmt.Errorf("'half_open_success_threshold' must be >= 1, got %d", halfOpenThreshold)
	}

	return &circuitBreakerOpts{
		failureThreshold:  int32(failureThreshold),
		recoveryDuration:  recoveryTime,
		halfOpenThreshold: int32(halfOpenThreshold),
	}, nil
}
