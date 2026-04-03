package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/nunoOliveiraqwe/torii/metrics"
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

func MetricsMiddleware(ctx context.Context, next http.HandlerFunc, _ Config) http.HandlerFunc {
	reportFunc := resolveReportFunc(ctx)
	return func(w http.ResponseWriter, r *http.Request) {
		if reportFunc == nil {
			next.ServeHTTP(w, r)
			return
		}
		logger := GetRequestLoggerFromContext(r)
		logger.Info("Applying metrics middleware for request")
		metric := initializeRequestMetrics(r)
		responseWriter := &responseWriterWithMetrics{ResponseWriter: w,
			reqMetrics: metric}
		startTime := time.Now()
		next.ServeHTTP(responseWriter, r)
		elapsedTime := time.Since(startTime)
		if err := r.Context().Err(); errors.Is(err, context.DeadlineExceeded) {
			metric.IsTimedOut = true
		}
		metric.LatencyMs = elapsedTime.Milliseconds()
		countryCode := r.Context().Value("country-code")
		if countryCode != nil {
			countryStr, ok := countryCode.(string)
			if ok {
				metric.Country = countryStr
			}
		}
		reportFunc(metric)
	}
}

func resolveReportFunc(ctx context.Context) metrics.MetricsReportFunc {
	port := ctx.Value("port")
	if port == nil || port == "" {
		zap.S().Warnf("Port not found in middleware options for metrics resolution")
		return nil
	}
	portStr, ok := port.(string)
	if !ok {
		_, isInt := port.(int)
		if !isInt {
			zap.S().Warnf("Port is not of type string")
			return nil
		}
		portStr = strconv.Itoa(port.(int))
	}
	pathStr := ""
	path := ctx.Value("path")
	if path != nil {
		pathStr2, ok := path.(string)
		if ok {
			pathStr = pathStr2
		}
	}
	conName := metrics.ProxyPathMetricsName(portStr, pathStr)

	mgrManager := ctx.Value(MgrKey)
	if mgrManager == nil {
		zap.S().Warnf("Mgr not found in middleware options for metrics resolution")
		return nil
	}
	mgrManagerCasted, ok := mgrManager.(*metrics.ConnectionMetricsManager)
	if !ok {
		zap.S().Warnf("Mgr is not of type SystemService")
		return nil
	}
	serverIdStr := ""
	serverId := ctx.Value("serverId")
	if serverId == nil {
		zap.S().Warnf("ServerId not found in middleware options for metrics resolution")
		return nil
	}
	serverIdStr = serverId.(string)
	return mgrManagerCasted.TrackMetricsForConnection(serverIdStr, conName)
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
