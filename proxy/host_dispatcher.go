package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/proxyutil"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

// VirtualHostDispatcher routes incoming requests to backend handlers based on
// the Host header, implementing name-based virtual hosting. Unmatched hosts
// fall through to the default handler if one is configured.
type VirtualHostDispatcher struct {
	routes   map[string]http.Handler
	default_ http.Handler
}

func (d *VirtualHostDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// strip port from Host so that "example.com:443" matches a route
	// keyed as "example.com". net.SplitHostPort fails when there is no
	// port, in which case we fall back to the raw Host value.
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	if handler, ok := d.routes[host]; ok {
		handler.ServeHTTP(w, r)
		return
	}
	if d.default_ != nil {
		d.default_.ServeHTTP(w, r)
		return
	}
	zap.S().Debugf("No route found for host %s", r.Host)
	http.Error(w, "no route", http.StatusBadGateway)
}

func buildHostDispatcher(ctx context.Context, defaultTarget *config.RouteTarget, routes []config.Route) (http.Handler, []string, []string, []RouteSnapshot, error) {
	zap.S().Infof("Building host dispatcher with %d routes, default: %v", len(routes), defaultTarget != nil)

	d := &VirtualHostDispatcher{
		routes: make(map[string]http.Handler),
	}
	var mwNames []string
	var backends []string
	var routeSnapshots []RouteSnapshot

	for _, route := range routes {
		if route.Host == "" {
			zap.S().Errorf("Route host cannot be empty, skipping")
			continue
		}
		routeCtx := context.WithValue(ctx, ctxkeys.Host, route.Host)
		handler, names, pathBackends, err := buildRouteHandler(routeCtx, route.Target)
		if err != nil {
			zap.S().Errorf("Failed to build handler for host %s: %v", route.Host, err)
			continue
		}
		d.routes[route.Host] = handler
		mwNames = append(mwNames, names...)
		backends = append(backends, route.Target.Backend)
		backends = append(backends, pathBackends...)
		routeSnapshots = append(routeSnapshots, buildRouteSnapshot(route.Host, route.Target))
		zap.S().Infof("Registered route for host %s with backend %s", route.Host, route.Target.Backend)
	}

	if defaultTarget != nil {
		defaultCtx := context.WithValue(ctx, ctxkeys.Host, "_default")
		handler, names, pathBackends, err := buildRouteHandler(defaultCtx, *defaultTarget)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to build default route handler: %w", err)
		}
		d.default_ = handler
		mwNames = append(mwNames, names...)
		backends = append(backends, defaultTarget.Backend)
		backends = append(backends, pathBackends...)
		routeSnapshots = append(routeSnapshots, buildRouteSnapshot("", *defaultTarget))
		zap.S().Infof("Registered default route with backend %s", defaultTarget.Backend)
	}

	if len(d.routes) == 0 && d.default_ == nil {
		return nil, nil, nil, nil, errors.New("no valid routes configured")
	}

	return d, mwNames, backends, routeSnapshots, nil
}

func buildRouteSnapshot(host string, target config.RouteTarget) RouteSnapshot {
	rs := RouteSnapshot{
		Host:        host,
		Backend:     target.Backend,
		Middlewares: middlewareNames(target.Middlewares),
	}
	for _, p := range target.Paths {
		rs.Paths = append(rs.Paths, PathSnapshot{
			Pattern:     p.Pattern,
			Backend:     p.Backend,
			Middlewares: middlewareNames(p.Middlewares),
		})
	}
	return rs
}

func buildRouteHandler(ctx context.Context, target config.RouteTarget) (http.Handler, []string, []string, error) {
	var baseHandler http.HandlerFunc
	if target.Backend != "" {
		proxy, err := buildHttpRevProxy(target.Backend, proxyutil.ProxyOptions{})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to build reverse proxy for backend %s: %w", target.Backend, err)
		}
		baseHandler = buildDefaultHttpHandler(proxy)
	} else {
		if !middleware.HasTerminatingMiddleware(target.Middlewares) {
			return nil, nil, nil, fmt.Errorf("route has no backend and no terminating middleware — nothing to serve requests")
		}
		baseHandler = func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no backend configured", http.StatusBadGateway)
		}
	}

	mwNames := middlewareNames(target.Middlewares)
	backends := make([]string, 0)
	if len(target.Paths) > 0 {
		pathHandler, pathMwNames, pathBackends, err := buildPathDispatcher(ctx, baseHandler, target.Paths)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to build path dispatcher for backend %s: %w", target.Backend, err)
		}
		mwNames = append(mwNames, pathMwNames...)
		backends = append(backends, pathBackends...)
		baseHandler = pathHandler.ServeHTTP
	}
	//global mw → route mw → path mw → proxy
	defaultHandler, err := buildMiddlewareChain(ctx, baseHandler, target.Middlewares)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to build middleware chain for backend %s: %w", target.Backend, err)
	}
	return defaultHandler, mwNames, backends, nil
}
