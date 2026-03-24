package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync/atomic"

	"github.com/nunoOliveiraqwe/micro-proxy/config"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/netutil"
	"github.com/nunoOliveiraqwe/micro-proxy/metrics"
	"github.com/nunoOliveiraqwe/micro-proxy/middleware"
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

func buildHttpServer(conf config.HTTPListener, mgr *metrics.ConnectionMetricsManager) (MicroHttpServer, error) {
	zap.S().Infof("Building HTTP server with configuration: %+v", conf)
	ifFace := "lo"
	if conf.Interface == "" {
		zap.S().Warn("No Interface in configuration, defaulting to loopback interface")
		i, err := netutil.GetLoopBackInterface()
		if err != nil {
			zap.S().Errorf("Failed to get loopback interface: %v", err)
			return nil, err
		}
		ifFace = i.Name
	}
	ipv4, ipv6, err := netutil.GetNetworkBindAddressesFromInterface(ifFace)
	if err != nil {
		zap.S().Errorf("Failed to get network bind addresses from interface %s: %v", conf.Interface, err)
		return nil, err
	}
	if conf.Bind&config.Ipv4Flag == 1 && ipv4 == "" {
		zap.S().Warnf("IPv4 bind interface %s does not have a valid IPv4 address", conf.Interface)
		return nil, fmt.Errorf("IPv4 bind interface %s does not have a valid IPv4 address", conf.Interface)
	}
	if conf.Bind&config.Ipv6Flag == 1 && ipv6 == "" {
		zap.S().Warnf("IPv6 bind interface %s does not have a valid IPv6 address", conf.Interface)
		return nil, fmt.Errorf("IPv6 bind interface %s does not have a valid IPv6 address", conf.Interface)
	}
	handler, mwNames, backends, err := buildHttpDispatcher(conf.Port, conf.Default, conf.Routes, mgr)
	if err != nil {
		zap.S().Errorf("Failed to build HTTP dispatcher: %v", err)
		return nil, err
	}

	mName := metrics.ProxyMetricsName(strconv.Itoa(conf.Port))
	zap.S().Infof("Built HTTP server proxy Ipv4=%s, Ipv6=%s, Port=%d", ipv4, ipv6, conf.Port)
	if conf.TLS != nil {
		return &MicroProxyHttpsServer{
			handler:           handler,
			readTimeout:       conf.ReadTimeout,
			readHeaderTimeout: conf.ReadHeaderTimeout,
			writeTimeout:      conf.WriteTimeout,
			idleTimeout:       conf.IdleTimeout,
			isStarted:         atomic.Bool{},
			bindPort:          conf.Port,
			iPV4BindInterface: ipv4,
			iPV6BindInterface: ipv6,
			useAcme:           conf.TLS.UseAcme,
			keyFilePath:       conf.TLS.Key,
			certFilepath:      conf.TLS.Cert,
			middlewareChain:   mwNames,
			metricsName:       mName,
			backends:          backends,
		}, nil
	}

	return &MicroProxyHttpServer{
		handler:           handler,
		readTimeout:       conf.ReadTimeout,
		readHeaderTimeout: conf.ReadHeaderTimeout,
		writeTimeout:      conf.WriteTimeout,
		idleTimeout:       conf.IdleTimeout,
		isStarted:         atomic.Bool{},
		bindPort:          conf.Port,
		iPV4BindInterface: ipv4,
		iPV6BindInterface: ipv6,
		middlewareChain:   mwNames,
		metricsName:       mName,
		backends:          backends,
	}, nil
}

func buildHttpDispatcher(port int, routeTarget *config.RouteTarget,
	routes []config.Route, mgr *metrics.ConnectionMetricsManager) (http.Handler, []string, []string, error) {
	zap.S().Infof("Building HTTP dispatcher with route target: %+v and routes: %+v", routeTarget, routes)
	if routeTarget != nil {
		return buildSingleRouteDispatcher(port, *routeTarget, mgr)
	}
	return buildMultiHostDispatcher(port, routes, mgr)
}

func buildSingleRouteDispatcher(port int, target config.RouteTarget, mgr *metrics.ConnectionMetricsManager) (http.Handler, []string, []string, error) {
	zap.S().Infof("Building single route dispatcher for target: %+v", target)
	proxy, err := buildHttpRevProxy(target.Backend)
	if err != nil {
		zap.S().Errorf("Failed to build reverse proxy for route with backend %s: %v", target.Backend, err)

	}
	defaultHandler := buildDefaultHttpHandler(proxy)
	handler, err := buildMiddlewareChain(port, defaultHandler, target.Middlewares, mgr)
	if err != nil {
		zap.S().Errorf("Failed to build middleware chain for route with backend %s: %v", target.Backend, err)
		return nil, nil, nil, err
	}
	return handler, middlewareNames(target.Middlewares), []string{target.Backend}, nil
}

func buildMultiHostDispatcher(port int, routes []config.Route, mgr *metrics.ConnectionMetricsManager) (http.Handler, []string, []string, error) {
	zap.S().Infof("Building multi-host dispatcher for routes: %+v", routes)
	if len(routes) == 0 {
		zap.S().Errorf("No routes provided for multi-host dispatcher")
		return nil, nil, nil, errors.New("no routes provided for multi-host dispatcher")
	}
	d := &MultiRouteHttpDispatcher{
		routes: make(map[string]http.Handler),
	}
	var mwNames []string
	var backends []string
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
		handler, err := buildMiddlewareChain(port, defaultHandler, route.Middlewares, mgr)
		if err != nil {
			zap.S().Errorf("Failed to build middleware chain for route with backend %s: %v", route.Backend, err)
			continue
		}
		d.routes[route.Host] = handler
		mwNames = append(mwNames, middlewareNames(route.Middlewares)...)
		backends = append(backends, route.Backend)
	}
	if len(d.routes) == 0 {
		zap.S().Errorf("No valid routes configured")
		return nil, nil, nil, fmt.Errorf("no valid routes configured")
	}
	return d, mwNames, backends, nil
}

func buildMiddlewareChain(port int, handler http.HandlerFunc, mwConfig []middleware.Config, mgr *metrics.ConnectionMetricsManager) (http.HandlerFunc, error) {
	if mwConfig == nil || len(mwConfig) == 0 {
		return handler, nil
	}
	next, err := middleware.ApplyMiddlewares(port, handler, mwConfig, mgr)
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

func rewriteProxyRequest(proxyUrl *url.URL) func(r *httputil.ProxyRequest) {
	return func(r *httputil.ProxyRequest) {
		r.SetURL(proxyUrl)
		r.SetXForwarded()
		r.Out.Header.Set("X-Origin-Host", proxyUrl.Host)
		zap.S().Debugf("Rewriting request to target: %s, X-Forwarded-For: %s", proxyUrl.String(), r.Out.Header.Get("X-Forwarded-For"))
	}
}

func middlewareNames(configs []middleware.Config) []string {
	names := make([]string, 0, len(configs))
	for _, c := range configs {
		names = append(names, c.Type)
	}
	return names
}
