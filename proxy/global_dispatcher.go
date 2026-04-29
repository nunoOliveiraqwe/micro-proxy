package proxy

import (
	"context"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

type GlobalDispatcher struct {
	internalHandlers map[string]http.HandlerFunc
	next             http.Handler
}

func (d *GlobalDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handler, ok := d.internalHandlers[r.URL.Path]; ok {
		zap.S().Debugf("Matched internal handler for path %s", r.URL.Path)
		handler(w, r)
		return
	}
	d.next.ServeHTTP(w, r)
}

func buildGlobalDispatcher(ctx context.Context, global *config.GlobalConfig, next http.Handler) (http.Handler, []string, error) {
	if global == nil {
		zap.S().Infof("No global dispatcher configuration provided. Skipping")
		return next, nil, nil
	}

	zap.S().Infof("Building global dispatcher with %d middlewares",
		len(global.Middlewares))

	d := &GlobalDispatcher{
		internalHandlers: make(map[string]http.HandlerFunc),
		next:             next,
	}

	if len(global.Middlewares) == 0 && global.TrustedProxies == nil {
		return d, nil, nil
	}

	var handler http.HandlerFunc = d.ServeHTTP

	var globalMwNames []middleware.Config

	if len(global.Middlewares) > 0 {
		wrapped, appliedMw, err := buildMiddlewareChain(ctx, handler, global.Middlewares, global.DisableDefaults)
		if err != nil {
			return nil, nil, err
		}
		handler = wrapped

		globalMwNames = appliedMw
	}

	handler = wrapTrustedProxies(ctx, handler, global.TrustedProxies)

	return handler, middlewareNames(globalMwNames), nil
}
