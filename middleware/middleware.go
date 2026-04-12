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
	Options map[string]interface{} `json:"options,omitempty"`
}

var registry Registry

func init() {
	registry = map[string]Func{
		"Metrics":          MetricsMiddleware,
		"RequestId":        RequestIDMiddleware,
		"RequestLog":       RequestLoggerMiddleware,
		"Headers":          HeadersMiddleware,
		"RateLimiter":      RateLimitMiddleware,
		"CountryBlock":     CountryBlockMiddleware,
		"IpBlock":          IpBlockMiddleware,
		"Redirect":         RedirectMiddleware,
		"BodySizeLimit":    BodySizeLimitMiddleware,
		"Timeout":          TimeoutMiddleware,
		"HoneyPot":         HoneyPotMiddleware,
		"UserAgentBlocker": UserAgentBlockMiddleware,
		"CircuitBreaker":   CircuitBreakerMiddleware,
		"Cors":             CorsMiddleware,
	}
}

func ApplyMiddlewares(ctx context.Context, handler http.HandlerFunc, middlewares []Config) (http.HandlerFunc, error) {
	if handler == nil {
		zap.S().Errorf("Handler cannot be nil when applying middleware chain")
		return nil, errors.New("handler cannot be nil when applying middleware chain")
	}
	middlewares = applyDefaultMiddlewares(middlewares)
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

var defaultMiddlewareOrder = []string{"RequestId", "RequestLog", "Metrics"}

var defaultMiddlewares = map[string]Func{
	"RequestId":  RequestIDMiddleware,
	"RequestLog": RequestLoggerMiddleware,
	"Metrics":    MetricsMiddleware,
}

func applyDefaultMiddlewares(middlewares []Config) []Config {
	zap.S().Debugf("Checking if default middlewares need to be applied, current middleware count: %d", len(middlewares))
	present := make(map[string]struct{})
	for _, v := range middlewares {
		if _, ok := defaultMiddlewares[v.Type]; ok {
			present[v.Type] = struct{}{}
		}
	}

	if len(present) == len(defaultMiddlewares) {
		//if the user was them, the order the user defined is applied
		zap.S().Debugf("All default middlewares are already configured, skipping applying defaults")
		return middlewares
	}

	var missing []Config
	for _, name := range defaultMiddlewareOrder {
		if _, already := present[name]; !already {
			missing = append(missing, Config{Type: name})
		}
	}

	if len(missing) == 0 {
		return middlewares
	}

	zap.S().Infof("Prepending %d missing default middleware(s) to chain", len(missing))

	// Prepend missing defaults so they run first (outermost), then the
	// user-configured chain follows.
	result := make([]Config, 0, len(missing)+len(middlewares))
	result = append(result, missing...)
	result = append(result, middlewares...)
	return result
}
