package middleware

import (
	"errors"
	"net/http"

	"go.uber.org/zap"
)

type Middleware func(next http.HandlerFunc, middlewareConf MiddlewareConfiguration) http.HandlerFunc

type MiddlewareRegistry = map[string]Middleware

type MiddlewareConfiguration struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"-"`
}

var registry MiddlewareRegistry

func init() {
	registry = map[string]Middleware{
		"Metrics":    MetricsMiddleware,
		"RequestId":  RequestIDMiddleware,
		"RequestLog": RequestLoggerMiddleware,
	}
}

func ApplyMiddlewares(handler http.HandlerFunc, middlewares []MiddlewareConfiguration) (http.HandlerFunc, error) {
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
		handler = middleware(handler, middlewares[i])
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

func GetMiddleware(key string) (Middleware, error) {
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
