package proxy

import (
	"context"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func buildMiddlewareChain(ctx context.Context, handler http.HandlerFunc, mwConfig []middleware.Config, disableDefaults bool) (http.HandlerFunc, error) {
	if len(mwConfig) == 0 && disableDefaults {
		return handler, nil
	}
	next, err := middleware.ApplyMiddlewares(ctx, handler, mwConfig, disableDefaults)
	if err != nil {
		zap.S().Errorf("Error applying middleware chain: %v", err)
		return nil, err
	}
	return next, nil
}

func middlewareNames(configs []middleware.Config) []string {
	names := make([]string, 0, len(configs))
	for _, c := range configs {
		names = append(names, c.Type)
	}
	return names
}
