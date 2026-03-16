package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/metrics"
	"go.uber.org/zap"
)

const METRICS_HANDLER_NAME = "metric_handler"

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

func MetricsMiddleware(next http.HandlerFunc, middlewareConf MiddlewareConfiguration) http.HandlerFunc {
	metricFunc := getConnectionMetrics(middlewareConf)
	if metricFunc == nil {
		zap.S().Warnf("Metrics middleware not configured properly. Skipping...")
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
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
		metricFunc(metric)
	}
}

func getConnectionMetrics(middlewareConf MiddlewareConfiguration) metrics.MetricsReportFunc {
	metricsHandlerT, ok := middlewareConf.Config[METRICS_HANDLER_NAME]
	if !ok {
		zap.S().Warnf("Metrics handler not found when configuring metrics middleware")
		return nil
	}
	metricsHandler, ok := metricsHandlerT.(*metrics.ConnectionMetricsHandler)
	if !ok {
		zap.S().Warnf("Metrics handler is not of type metrics.ConnectionMetricsHandler")
		return nil
	}
	metricsNameT, ok := middlewareConf.Config["name"]
	if !ok {
		zap.S().Warnf("Metrics name not found when configuring metrics middleware")
		return nil
	}
	metricsName, ok := metricsNameT.(string)
	if !ok {
		zap.S().Warnf("Metrics name is not of type string")
		return nil
	}
	zap.S().Debugf("Creating metrics handler for connection %s", metricsName)
	return metricsHandler.NewConnectionMetric(metricsName)
}

func initializeRequestMetrics(r *http.Request) *metrics.RequestMetric {
	return &metrics.RequestMetric{
		BytesReceived: r.ContentLength,
		BytesSent:     0,
		IsTimedOut:    false,
	}
}
