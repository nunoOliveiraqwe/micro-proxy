package proxy

import (
	"context"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/config"
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

func buildGlobalDispatcher(ctx context.Context, global *config.GlobalConfig, next http.Handler) (http.Handler, error) {
	if global == nil {
		zap.S().Infof("No global dispatcher configuration provided. Skipping")
		return next, nil
	}

	zap.S().Infof("Building global dispatcher with %d middlewares",
		len(global.Middlewares))

	d := &GlobalDispatcher{
		internalHandlers: make(map[string]http.HandlerFunc),
		next:             next,
	}

	if len(global.Middlewares) == 0 {
		return d, nil
	}

	wrapped, err := buildMiddlewareChain(ctx, d.ServeHTTP, global.Middlewares)
	if err != nil {
		return nil, err
	}
	return wrapped, nil
}
