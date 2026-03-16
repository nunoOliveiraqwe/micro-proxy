package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"

	"github.com/nunoOliveiraqwe/micro-proxy/configuration"
	"github.com/nunoOliveiraqwe/micro-proxy/middleware"
	"github.com/nunoOliveiraqwe/micro-proxy/util"
	"go.uber.org/zap"
)

type MultiRouteHttpDispatcher struct {
	routes map[string]http.Handler
}

func (d *MultiRouteHttpDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handler, ok := d.routes[r.Host]; ok { //TODO, maybe build a trie tree matcher for this
		handler.ServeHTTP(w, r)
		return
	}
	zap.S().Debugf("No route found for host %s", r.Host)
	http.Error(w, "no route", http.StatusBadGateway)
}

func buildHttpServer(config configuration.HTTPListener) (MicroHttpServer, error) {
	zap.S().Infof("Building HTTP server with configuration: %+v", config)
	ipv4, ipv6, err := util.GetNetworkBindAddressesFromInterface(config.Interface)
	if err != nil {
		zap.S().Errorf("Failed to get network bind addresses from interface %s: %v", config.Interface, err)
		return nil, err
	}
	if config.Bind&configuration.Ipv4Flag == 1 && ipv4 == "" {
		zap.S().Warnf("IPv4 bind interface %s does not have a valid IPv4 address", config.Interface)
		return nil, fmt.Errorf("IPv4 bind interface %s does not have a valid IPv4 address", config.Interface)
	}
	if config.Bind&configuration.Ipv6Flag == 1 && ipv6 == "" {
		zap.S().Warnf("IPv6 bind interface %s does not have a valid IPv6 address", config.Interface)
		return nil, fmt.Errorf("IPv6 bind interface %s does not have a valid IPv6 address", config.Interface)
	}
	handler, err := buildHttpDispatcher(config.Default, config.Routes)
	if err != nil {
		zap.S().Errorf("Failed to build HTTP dispatcher: %v", err)
		return nil, err
	}
	srv := &http.Server{
		Handler:                      handler,
		DisableGeneralOptionsHandler: false,
		ReadTimeout:                  config.ReadTimeout,
		ReadHeaderTimeout:            config.ReadHeaderTimeout,
		WriteTimeout:                 config.WriteTimeout,
		IdleTimeout:                  config.IdleTimeout,
	}

	if config.TLS != nil {
		return &MicroProxyHttpsServer{
			httpServer:        srv,
			isStarted:         atomic.Bool{},
			bindPort:          config.Port,
			iPV4BindInterface: ipv4,
			iPV6BindInterface: ipv6,
			useAcme:           config.TLS.UseAcme,
			keyFilePath:       config.TLS.Key,
			certFilepath:      config.TLS.Cert,
		}, nil
	}

	return &MicroProxyHttpServer{
		httpServer:        srv,
		isStarted:         atomic.Bool{},
		bindPort:          config.Port,
		iPV4BindInterface: ipv4,
		iPV6BindInterface: ipv4,
	}, nil
}

func buildHttpDispatcher(routeTarget *configuration.RouteTarget, routes []configuration.Route) (http.Handler, error) {
	zap.S().Infof("Building HTTP dispatcher with route target: %+v and routes: %+v", routeTarget, routes)
	if routeTarget != nil {
		return buildSingleRouteDispatcher(*routeTarget)
	}
	return buildMultiHostDispatcher(routes)
}

func buildSingleRouteDispatcher(target configuration.RouteTarget) (http.Handler, error) {
	zap.S().Infof("Building single route dispatcher for target: %+v", target)
	proxy, err := buildHttpRevProxy(target.Backend)
	if err != nil {
		zap.S().Errorf("Failed to build reverse proxy for route with backend %s: %v", target.Backend, err)

	}
	defaultHandler := buildDefaultHttpHandler(proxy)
	handler, err := buildMiddlewareChain(defaultHandler, target.Middlewares)
	if err != nil {
		zap.S().Errorf("Failed to build middleware chain for route with backend %s: %v", target.Backend, err)
		return nil, err
	}
	return handler, nil
}

func buildMultiHostDispatcher(routes []configuration.Route) (http.Handler, error) {
	zap.S().Infof("Building multi-host dispatcher for routes: %+v", routes)
	if len(routes) == 0 {
		zap.S().Errorf("No routes provided for multi-host dispatcher")
		return nil, errors.New("no routes provided for multi-host dispatcher")
	}
	d := &MultiRouteHttpDispatcher{
		routes: make(map[string]http.Handler),
	}

	for _, route := range routes {
		zap.S().Debugf("Building route for host %s with backend %s", route.Host, route.Backend)
		if route.Host == "" {
			zap.S().Errorf("Route host cannot be empty")
			continue
		}
		proxy, err := buildHttpRevProxy(route.Backend)
		if err != nil {
			zap.S().Errorf("Failed to build reverse proxy for route with backend %s: %v", route.Backend, err)
			continue
		}
		defaultHandler := buildDefaultHttpHandler(proxy)
		handler, err := buildMiddlewareChain(defaultHandler, route.Middlewares)
		if err != nil {
			zap.S().Errorf("Failed to build middleware chain for route with backend %s: %v", route.Backend, err)
			continue
		}
		d.routes[route.Host] = handler
	}
	if len(d.routes) == 0 {
		zap.S().Errorf("No valid routes configured")
		return nil, fmt.Errorf("no valid routes configured")
	}
	return d, nil
}

func buildMiddlewareChain(handler http.HandlerFunc, configuration []middleware.MiddlewareConfiguration) (http.HandlerFunc, error) {
	if configuration == nil || len(configuration) == 0 {
		return handler, nil
	}
	next, err := middleware.ApplyMiddlewares(handler, configuration)
	if err != nil {
		zap.S().Errorf("Error applying middleware chain: %v", err)
		return nil, err
	}
	return next, nil
}

func buildDefaultHttpHandler(proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Debugf("Proxying request to target: %s, X-Forwarded-For: %s",
			r.URL.String(), r.Header.Get("X-Forwarded-For"))
		proxy.ServeHTTP(w, r)
	}
}

func buildHttpRevProxy(backend string) (*httputil.ReverseProxy, error) {
	zap.S().Infof("Building proxy for HTTP server with target URL: %s", backend)
	parsedUrl, err := url.Parse(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: rewriteProxyRequest(parsedUrl),
	}
	return proxy, nil
}

func rewriteProxyRequest(proxyUrl *url.URL) func(r *httputil.ProxyRequest) { //could be a re-write header middleware, but it's simpler like this
	return func(r *httputil.ProxyRequest) {
		r.SetURL(proxyUrl)
		r.SetXForwarded()
		r.Out.Header.Set("X-Origin-Host", proxyUrl.Host)
		zap.S().Debugf("Rewriting request to target: %s, X-Forwarded-For: %s", proxyUrl.String(), r.Out.Header.Get("X-Forwarded-For"))
	}
}
