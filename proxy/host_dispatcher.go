package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/config"
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
	if handler, ok := d.routes[r.Host]; ok {
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
		handler, names, err := buildRouteHandler(ctx, route.Target)
		if err != nil {
			zap.S().Errorf("Failed to build handler for host %s: %v", route.Host, err)
			continue
		}
		d.routes[route.Host] = handler
		mwNames = append(mwNames, names...)
		backends = append(backends, route.Target.Backend)
		routeSnapshots = append(routeSnapshots, buildRouteSnapshot(route.Host, route.Target))
		zap.S().Infof("Registered route for host %s with backend %s", route.Host, route.Target.Backend)
	}

	if defaultTarget != nil {
		handler, names, err := buildRouteHandler(ctx, *defaultTarget)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to build default route handler: %w", err)
		}
		d.default_ = handler
		mwNames = append(mwNames, names...)
		backends = append(backends, defaultTarget.Backend)
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
			Middlewares: middlewareNames(p.Middlewares),
		})
	}
	return rs
}

func buildRouteHandler(ctx context.Context, target config.RouteTarget) (http.Handler, []string, error) {
	proxy, err := buildHttpRevProxy(target.Backend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build reverse proxy for backend %s: %w", target.Backend, err)
	}
	baseHandler := buildDefaultHttpHandler(proxy)

	defaultHandler, err := buildMiddlewareChain(ctx, baseHandler, target.Middlewares)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build middleware chain for backend %s: %w", target.Backend, err)
	}
	mwNames := middlewareNames(target.Middlewares)

	if len(target.Paths) > 0 {
		pathHandler, pathMwNames, err := buildPathDispatcher(ctx, defaultHandler, target.Paths, baseHandler)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build path dispatcher for backend %s: %w", target.Backend, err)
		}
		mwNames = append(mwNames, pathMwNames...)
		return pathHandler, mwNames, nil
	}

	return defaultHandler, mwNames, nil
}
