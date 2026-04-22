package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logEntryContextKey = ctxkeys.Logger

type zapLogFormatter struct {
	logger *zap.Logger
}

func newZapLogFormatter(accessLogFileName string) *zapLogFormatter {
	conf := zap.NewProductionConfig()
	if accessLogFileName != "" {
		conf.OutputPaths = []string{"stdout", accessLogFileName}
	}
	conf.DisableCaller = true
	conf.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := conf.Build()
	if err != nil {
		logger = zap.NewNop()
		zap.S().Errorf("Failed to initialize request logger: %v", err)
	}
	return &zapLogFormatter{
		logger: logger,
	}
}

type accessLogResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

func (w *accessLogResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = statusCode
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *accessLogResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.statusCode = http.StatusOK
		w.wroteHeader = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *accessLogResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *accessLogResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (z *zapLogFormatter) newRequestLogger(r *http.Request) *zap.Logger {
	ctx := r.Context()
	reqId := ""
	if ctx != nil {
		reqId = GetRequestIDFromContext(ctx)
	}
	return z.logger.With(
		zap.String("request_id", reqId),
		zap.String("method", r.Method),
		zap.String("host", r.Host),
		zap.String("url", r.URL.String()),
		zap.String("remote_addr", r.RemoteAddr),
		zap.String("user_agent", r.UserAgent()),
	)
}

func GetRequestLoggerFromContext(r *http.Request) *zap.Logger {
	ctx := r.Context()
	if ctx == nil || ctx.Value(logEntryContextKey) == nil {
		return zap.L()
	}
	log := ctx.Value(logEntryContextKey)
	if logEntry, ok := log.(*zap.Logger); ok {
		return logEntry
	}
	return zap.L()
}

func RequestLoggerMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	formatterPath := parseConfig(conf)
	formatter := newZapLogFormatter(formatterPath)
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log := formatter.newRequestLogger(r)

		ctx := context.WithValue(r.Context(), logEntryContextKey, log)
		r = r.WithContext(ctx)

		aw := &accessLogResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(aw, r)

		latency := time.Since(start)

		// Choose log level based on status code
		fields := []zap.Field{
			zap.Int("status", aw.statusCode),
			zap.Duration("latency", latency),
			zap.Int64("bytes_sent", aw.bytesWritten),
			zap.Int64("bytes_received", r.ContentLength),
		}
		if aw.statusCode >= 500 {
			log.Error("Request completed", fields...)
		} else if aw.statusCode >= 400 {
			log.Warn("Request completed", fields...)
		} else {
			log.Info("Request completed", fields...)
		}
	}
}

func parseConfig(conf Config) string {
	path, ok := conf.Options["request-log-path"]
	if !ok {
		return ""
	}
	if pathStr, ok := path.(string); ok {
		return pathStr
	}
	zap.S().Warn("RequestLoggerMiddleware: request log path is not a string, defaulting to stdout only")
	return ""
}
