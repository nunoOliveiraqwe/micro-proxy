package middleware

import (
	"context"
	"errors"
	"net/http"

	"go.uber.org/zap"
)

type Func func(ctx context.Context, next http.HandlerFunc, middlewareConf Config) http.HandlerFunc

type RegistryEntry struct {
	Fn         Func
	Terminates bool //indicates that the middleware terminates the handling, e.g, redirect
}

type Registry = map[string]RegistryEntry

type Config struct {
	Type    string                 `json:"type"`
	Options map[string]interface{} `json:"options,omitempty"`
}

var registry Registry

func init() {
	registry = map[string]RegistryEntry{
		"Metrics":          {Fn: MetricsMiddleware},
		"RequestId":        {Fn: RequestIDMiddleware},
		"RequestLog":       {Fn: RequestLoggerMiddleware},
		"Headers":          {Fn: HeadersMiddleware},
		"RateLimiter":      {Fn: RateLimitMiddleware},
		"CountryBlock":     {Fn: CountryBlockMiddleware},
		"IpBlock":          {Fn: IpBlockMiddleware},
		"Redirect":         {Fn: RedirectMiddleware, Terminates: true},
		"BodySizeLimit":    {Fn: BodySizeLimitMiddleware},
		"Timeout":          {Fn: TimeoutMiddleware},
		"HoneyPot":         {Fn: HoneyPotMiddleware},
		"UserAgentBlocker": {Fn: UserAgentBlockMiddleware},
		"CircuitBreaker":   {Fn: CircuitBreakerMiddleware},
		"Cors":             {Fn: CorsMiddleware},
		"StaticResponse":   {Fn: StaticResponseMiddleware, Terminates: true},
	}
}

func ApplyMiddlewares(ctx context.Context, handler http.HandlerFunc, middlewares []Config, disableDefaults bool) (http.HandlerFunc, error) {
	if handler == nil {
		zap.S().Errorf("Handler cannot be nil when applying middleware chain")
		return nil, errors.New("handler cannot be nil when applying middleware chain")
	}
	if !disableDefaults {
		middlewares = applyDefaultMiddlewares(middlewares)
	}
	zap.S().Debugf("Applying middleware chain with size %d", len(middlewares))
	for i := len(middlewares) - 1; i >= 0; i-- {
		entry, err := GetMiddleware(middlewares[i].Type)
		if err != nil {
			zap.S().Errorf("Error applying middleware of type %s: %v", middlewares[i].Type, err)
			return nil, err
		}
		if middlewares[i].Options == nil {
			zap.S().Warnf("Middleware options for middleware of type %s is nil. Initializing it as an empty map", middlewares[i].Type)
			middlewares[i].Options = make(map[string]interface{})
		}
		handler = entry.Fn(ctx, handler, middlewares[i])
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

func GetMiddleware(key string) (RegistryEntry, error) {
	if key == "" {
		return RegistryEntry{}, errors.New("middleware key cannot be empty")
	}
	entry, exists := registry[key]
	if !exists {
		return RegistryEntry{}, errors.New("middleware not found")
	}
	return entry, nil
}

func GetAvailableMiddlewares() []string {
	middlewares := make([]string, 0, len(registry))
	for key := range registry {
		middlewares = append(middlewares, key)
	}
	return middlewares
}

func HasTerminatingMiddleware(middlewares []Config) bool {
	for _, m := range middlewares {
		if entry, exists := registry[m.Type]; exists && entry.Terminates {
			return true
		}
	}
	return false
}

var defaultMiddlewareOrder = []string{"RequestId", "RequestLog", "Metrics"}

var defaultMiddlewares = map[string]struct{}{
	"RequestId":  {},
	"RequestLog": {},
	"Metrics":    {},
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

	result := make([]Config, 0, len(missing)+len(middlewares))
	result = append(result, missing...)
	result = append(result, middlewares...)
	return result
}
