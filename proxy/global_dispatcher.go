package proxy

import (
	"context"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"go.uber.org/zap"
)

type GlobalDispatcher struct {
	globalConfig       *config.GlobalConfig
	globalMwNames      []string
	registeredHandlers map[int]http.HandlerFunc
	globalChain        http.HandlerFunc
}

func (d *GlobalDispatcher) registerHandler(port int, next http.HandlerFunc) http.HandlerFunc {
	if d.globalConfig == nil {
		return next
	} else if d.globalChain == nil {
		return next
	}
	d.registeredHandlers[port] = next
	return func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), ctxkeys.Port, port))
		d.globalChain(w, r)
	}
}

func (d *GlobalDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handler, exists := d.registeredHandlers[r.Context().Value(ctxkeys.Port).(int)]; exists {
		handler(w, r)
	} else {
		zap.S().Errorf("No handler registered for port %d", r.Context().Value(ctxkeys.Port).(int))
		http.Error(w, "", http.StatusNotFound)
	}
}

func initGlobalDispatcher(ctx context.Context, global *config.GlobalConfig) (*GlobalDispatcher, error) {
	if global == nil {
		zap.S().Infof("No global dispatcher configuration provided. Skipping")
		return &GlobalDispatcher{}, nil
	}

	zap.S().Infof("Initializing global dispatcher with %d middlewares", len(global.Middlewares))

	if len(global.Middlewares) == 0 && global.TrustedProxies == nil {
		return &GlobalDispatcher{}, nil
	}

	d := &GlobalDispatcher{
		globalConfig:       global,
		registeredHandlers: make(map[int]http.HandlerFunc),
	}
	handler := d.ServeHTTP

	if len(global.Middlewares) > 0 {
		ctx = context.WithValue(ctx, ctxkeys.ServerID, "global")
		ctx = context.WithValue(ctx, ctxkeys.OverrideMetricsName, "global")

		wrapped, appliedMw, err := buildMiddlewareChain(ctx, handler, global.Middlewares, global.DisableDefaults)
		if err != nil {
			return nil, err
		}
		handler = wrapped
		d.globalMwNames = middlewareNames(appliedMw)
	}

	handler = wrapTrustedProxies(ctx, handler, global.TrustedProxies)
	d.globalChain = handler
	return d, nil
}
