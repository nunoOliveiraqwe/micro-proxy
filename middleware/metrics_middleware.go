package middleware

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/metrics"
	"go.uber.org/zap"
)

type responseWriterWithMetrics struct {
	http.ResponseWriter
	reqMetrics *metrics.RequestMetric
}

func (w *responseWriterWithMetrics) WriteHeader(statusCode int) {
	w.reqMetrics.Is2xxResponse = statusCode >= 200 && statusCode < 300
	w.reqMetrics.Is3xxResponse = statusCode >= 300 && statusCode < 400
	w.reqMetrics.Is4xxResponse = statusCode >= 400 && statusCode < 500
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWithMetrics) Write(b []byte) (int, error) {
	w.reqMetrics.BytesSent = int64(len(b))
	return w.ResponseWriter.Write(b)
}

func MetricsMiddleware(next http.HandlerFunc, _ Config) http.HandlerFunc {
	var (
		once       sync.Once
		reportFunc metrics.MetricsReportFunc
	)

	return func(w http.ResponseWriter, r *http.Request) {
		// Lazily resolve the report func on the first request – after that
		// the cached value is reused for every subsequent request.
		once.Do(func() {
			reportFunc = resolveReportFunc(r)
		})

		if reportFunc == nil {
			next.ServeHTTP(w, r)
			return
		}

		logger := getRequestLoggerFromContext(r)
		logger.Debug("Recording metrics for request")
		metric := initializeRequestMetrics(r)
		responseWriter := &responseWriterWithMetrics{ResponseWriter: w,
			reqMetrics: metric}
		startTime := time.Now()
		next.ServeHTTP(responseWriter, r)
		elapsedTime := time.Since(startTime)
		if err := r.Context().Err(); err == context.DeadlineExceeded {
			metric.IsTimedOut = true
		}
		metric.LatencyMs = elapsedTime.Milliseconds()
		reportFunc(metric)
	}
}

func resolveReportFunc(r *http.Request) metrics.MetricsReportFunc {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		zap.S().Warnf("Could not determine local address for metrics resolution")
		return nil
	}
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		zap.S().Warnf("Could not parse local address %s for metrics: %v", addr.String(), err)
		return nil
	}
	return metrics.GlobalMetricsManager.GetReportFuncByPort(port)
}

func initializeRequestMetrics(r *http.Request) *metrics.RequestMetric {
	return &metrics.RequestMetric{
		BytesReceived: r.ContentLength,
		BytesSent:     0,
		IsTimedOut:    false,
	}
}
