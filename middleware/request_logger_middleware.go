package middleware

import (
	"context"
	"net/http"

	"go.uber.org/zap"
)

var logEntryContextKey = "logEntry"

type zapLogFormatter struct {
	logger *zap.Logger
}

func newZapLogFormatter() *zapLogFormatter {
	return &zapLogFormatter{
		logger: zap.L(),
	}
}

func (z *zapLogFormatter) LogRequest(r *http.Request) {
	ctx := r.Context()
	reqId := ""
	if ctx != nil {
		reqIdValue := ctx.Value(requestIdContextKey)
		if reqIdValue != nil {
			if reqIdStr, ok := reqIdValue.(string); ok {
				reqId = reqIdStr
			}
		}
	} else {
		ctx = context.Background()
	}
	log := z.logger.With(
		zap.String("method", r.Method),
		zap.String("url", r.URL.String()),
		zap.String("request_id", reqId),
		zap.String("user_agent", r.UserAgent()),
		zap.String("remote_addr", r.RemoteAddr),
		zap.String("host", r.Host),
	)
	log.Info("Incoming request")
	ctx = context.WithValue(ctx, logEntryContextKey, log)
	r.WithContext(ctx)
}

func getRequestLoggerFromContext(r *http.Request) *zap.Logger {
	ctx := r.Context()
	if ctx == nil || ctx.Value(logEntryContextKey) == nil {
		return zap.NewNop()
	}
	log := ctx.Value(logEntryContextKey)
	if logEntry, ok := log.(*zap.Logger); ok {
		return logEntry
	}
	return zap.NewNop()
}

func RequestLoggerMiddleware(next http.HandlerFunc, middleware MiddlewareConfiguration) http.HandlerFunc {
	newZapLogFormatter := newZapLogFormatter()
	return func(w http.ResponseWriter, r *http.Request) {
		newZapLogFormatter.LogRequest(r)
		next.ServeHTTP(w, r)
	}
}
