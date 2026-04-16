package middleware

import (
	"context"
	"net/http"

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
	conf.DisableCaller = false
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

func (z *zapLogFormatter) LogRequest(r *http.Request) {
	ctx := r.Context()
	reqId := ""
	if ctx != nil {
		reqId = GetRequestIDFromContext(ctx)
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
	*r = *r.WithContext(ctx)
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
	newZapLogFormatter := newZapLogFormatter(formatterPath)
	return func(w http.ResponseWriter, r *http.Request) {
		newZapLogFormatter.LogRequest(r)
		next.ServeHTTP(w, r)
	}
}

func parseConfig(conf Config) string {
	path, ok := conf.Options["request-log-path"]
	if !ok {
		zap.S().Info("RequestLoggerMiddleware: no request log path specified, defaulting to stdout only")
		return ""
	}
	if pathStr, ok := path.(string); ok {
		zap.S().Info("RequestLoggerMiddleware: using request log path from configuration: %s", pathStr)
		return pathStr
	}
	zap.S().Warn("RequestLoggerMiddleware: request log path is not a string, defaulting to stdout only")
	return ""
}
