package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/metrics"
	"go.uber.org/zap"
)

type responseWriterWithMetrics struct {
	http.ResponseWriter
	reqMetrics  *metrics.RequestMetric
	wroteHeader bool
}

func (w *responseWriterWithMetrics) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.reqMetrics.StatusCode = statusCode
	w.reqMetrics.Is2xxResponse = statusCode >= 200 && statusCode < 300
	w.reqMetrics.Is3xxResponse = statusCode >= 300 && statusCode < 400
	w.reqMetrics.Is4xxResponse = statusCode >= 400 && statusCode < 500
	w.reqMetrics.Is5xxResponse = statusCode >= 500 && statusCode < 600
	w.ResponseWriter.WriteHeader(statusCode)
	w.wroteHeader = true
}

func (w *responseWriterWithMetrics) Write(b []byte) (int, error) {
	w.reqMetrics.BytesSent = int64(len(b))
	//htto/server.go writes the header when calling write
	// if I write twice, i get a superfluous response.WriteHeader call log which annoys me to no end
	n, err := w.ResponseWriter.Write(b)
	if err == nil {
		w.wroteHeader = true
		if w.reqMetrics.StatusCode == 0 {
			w.reqMetrics.StatusCode = 200
		}
		w.reqMetrics.Is2xxResponse = true
	}
	return n, err
}

func (w *responseWriterWithMetrics) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *responseWriterWithMetrics) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func MetricsMiddleware(next http.HandlerFunc, conf Config) http.HandlerFunc {
	reportFunc := resolveReportFunc(conf)
	return func(w http.ResponseWriter, r *http.Request) {
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

func resolveReportFunc(conf Config) metrics.MetricsReportFunc {
	port := conf.Options["port"]
	if port == "" {
		zap.S().Warnf("Port not found in middleware options for metrics resolution")
		return nil
	}
	portStr, ok := port.(string)
	if !ok {
		zap.S().Warnf("Port is not of type string")
		return nil
	}
	conName := metrics.ProxyMetricsName(portStr)

	mgrManager, exists := conf.Options[MgrKey]
	if !exists {
		zap.S().Warnf("Mgr not found in middleware options for metrics resolution")
		return nil
	}
	mgrManagerCasted, ok := mgrManager.(*metrics.ConnectionMetricsManager)
	if !ok {
		zap.S().Warnf("Mgr is not of type SystemService")
		return nil
	}
	return mgrManagerCasted.TrackMetricsForConnection(conName)
}

func initializeRequestMetrics(r *http.Request) *metrics.RequestMetric {
	return &metrics.RequestMetric{
		RemoteAddress: r.RemoteAddr,
		BytesReceived: r.ContentLength,
		BytesSent:     0,
		IsTimedOut:    false,
		Path:          r.URL.Path,
		Method:        r.Method,
	}
}
