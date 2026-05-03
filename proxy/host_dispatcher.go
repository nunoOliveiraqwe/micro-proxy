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
	routeTrie *HostTrie
	default_  http.Handler
}

func (d *VirtualHostDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host

	host, _, err := net.SplitHostPort(host)
	if err != nil {
		zap.S().Errorf("Failed to parse host %s: %v", host, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	handler := d.routeTrie.Contains(host)

	if handler != nil {
		handler.ServeHTTP(w, r)
		return
	}

	if d.default_ != nil {
		d.default_.ServeHTTP(w, r)
		return
	}
	zap.S().Debugf("No route found for host %s", r.Host)
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

func buildHostDispatcher(ctx context.Context, defaultTarget *config.RouteTarget, routes []config.Route) (http.Handler, []string, []RouteSnapshot, error) {
	zap.S().Infof("Building host dispatcher with %d routes, default: %v", len(routes), defaultTarget != nil)

	d := &VirtualHostDispatcher{
		routeTrie: NewHostTrie(),
	}
	var backends []string
	var routeSnapshots []RouteSnapshot

	for _, route := range routes {
		if route.Host == "" {
			zap.S().Errorf("Route host cannot be empty, skipping")
			continue
		}
		routeCtx := context.WithValue(ctx, ctxkeys.Host, route.Host)
		handler, pathBackends, snapshot, err := buildRouteHandler(routeCtx, route.Target)
		if err != nil {
			zap.S().Errorf("Failed to build handler for host %s: %v", route.Host, err)
			continue
		}
		d.routeTrie.InsertHost(route.Host, handler)
		backends = append(backends, route.Target.Backend.Address)
		backends = append(backends, pathBackends...)
		snapshot.Host = route.Host
		routeSnapshots = append(routeSnapshots, snapshot)
		zap.S().Infof("Registered route for host %s with backend %s", route.Host, route.Target.Backend.Address)
	}

	if defaultTarget != nil {
		defaultCtx := context.WithValue(ctx, ctxkeys.Host, "_default")
		handler, pathBackends, snapshot, err := buildRouteHandler(defaultCtx, *defaultTarget)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to build default route handler: %w", err)
		}
		d.default_ = handler
		backends = append(backends, defaultTarget.Backend.Address)
		backends = append(backends, pathBackends...)
		routeSnapshots = append(routeSnapshots, snapshot)
		zap.S().Infof("Registered default route with backend %s", defaultTarget.Backend.Address)
	}

	if !d.routeTrie.HasAnyEntry() && d.default_ == nil {
		return nil, nil, nil, errors.New("no valid routes configured")
	}

	return d, backends, routeSnapshots, nil
}

func buildRouteHandler(ctx context.Context, target config.RouteTarget) (http.Handler, []string, RouteSnapshot, error) {
	var baseHandler http.HandlerFunc
	if target.Backend.Address != "" {
		proxy, err := buildHttpRevProxy(target.Backend.Address, proxyutil.ProxyOptions{
			ReplaceHostHeader: target.Backend.ReplaceHostHeader,
		})
		if err != nil {
			return nil, nil, RouteSnapshot{}, fmt.Errorf("failed to build reverse proxy for backend %s: %w", target.Backend.Address, err)
		}
		baseHandler = buildDefaultHttpHandler(proxy)
	} else {
		if !middleware.HasTerminatingMiddleware(target.Middlewares) {
			return nil, nil, RouteSnapshot{}, fmt.Errorf("route has no backend and no terminating middleware — nothing to serve requests")
		}
		baseHandler = func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "no backend configured", http.StatusBadGateway)
		}
	}

	backends := make([]string, 0)
	var pathSnapshots []PathSnapshot
	if len(target.Paths) > 0 {
		pathHandler, pathBackends, pSnaps, err := buildPathDispatcher(ctx, baseHandler, target.Paths)
		if err != nil {
			return nil, nil, RouteSnapshot{}, fmt.Errorf("failed to build path dispatcher for backend %s: %w", target.Backend.Address, err)
		}
		backends = append(backends, pathBackends...)
		pathSnapshots = pSnaps
		baseHandler = pathHandler.ServeHTTP
	}

	//global mw → route mw → path mw → proxy
	defaultHandler, appliedMw, err := buildMiddlewareChain(ctx, baseHandler, target.Middlewares, target.DisableDefaults)
	if err != nil {
		return nil, nil, RouteSnapshot{}, fmt.Errorf("failed to build middleware chain for backend %s: %w", target.Backend.Address, err)
	}

	defaultHandler = wrapTrustedProxies(ctx, defaultHandler, target.TrustedProxies)

	snapshot := RouteSnapshot{
		Backend:     target.Backend.Address,
		Middlewares: middlewareNames(appliedMw),
		Paths:       pathSnapshots,
	}

	return defaultHandler, backends, snapshot, nil
}
