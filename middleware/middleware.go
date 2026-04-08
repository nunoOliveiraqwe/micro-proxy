package middleware

import (
	"context"
	"errors"
	"net/http"

	"go.uber.org/zap"
)

const MgrKey = "metricsManager"

type Func func(ctx context.Context, next http.HandlerFunc, middlewareConf Config) http.HandlerFunc

type Registry = map[string]Func

type Config struct {
	Type    string                 `json:"type"`
	Options map[string]interface{} `json:"-"`
}

var registry Registry

func init() {
	registry = map[string]Func{
		"Metrics":        MetricsMiddleware,
		"RequestId":      RequestIDMiddleware,
		"RequestLog":     RequestLoggerMiddleware,
		"Headers":        HeadersMiddleware,
		"RateLimiter":    RateLimitMiddleware,
		"CountryBlock":   CountryBlockMiddleware,
		"IpBlock":        IpBlockMiddleware,
		"Redirect":       RedirectMiddleware,
		"BodySizeLimit":  BodySizeLimitMiddleware,
		"Timeout":        TimeoutMiddleware,
		"HoneyPot":       HoneyPotMiddleware,
		"CircuitBreaker": CircuitBreakerMiddleware,
	}
}

func ApplyMiddlewares(ctx context.Context, handler http.HandlerFunc, middlewares []Config) (http.HandlerFunc, error) {
	if handler == nil {
		zap.S().Errorf("Handler cannot be nil when applying middleware chain")
		return nil, errors.New("handler cannot be nil when applying middleware chain")
	}
	zap.S().Debugf("Applying middleware chain with size %d", len(middlewares))
	for i := len(middlewares) - 1; i >= 0; i-- {
		middleware, err := GetMiddleware(middlewares[i].Type)
		if err != nil {
			zap.S().Errorf("Error applying middleware of type %s: %v", middlewares[i].Type, err)
			return nil, err
		}
		if middlewares[i].Options == nil {
			zap.S().Warnf("Middleware options for middleware of type %s is nil. Initializing it as an empty map", middlewares[i].Type)
			middlewares[i].Options = make(map[string]interface{})
		}
		handler = middleware(ctx, handler, middlewares[i])
	}
	return handler, nil
}

func MiddlewareExists(key string) bool {
	if key == "" {
		return false
	}
	_, exists := registry[key]
	return exists
}

func GetMiddleware(key string) (Func, error) {
	if key == "" {
		return nil, errors.New("middleware key cannot be empty")
	}
	middleware, exists := registry[key]
	if !exists {
		return nil, errors.New("middleware not found")
	}
	return middleware, nil
}

func GetAvailableMiddlewares() []string {
	middlewares := make([]string, 0, len(registry))
	for key := range registry {
		middlewares = append(middlewares, key)
	}
	return middlewares
}
