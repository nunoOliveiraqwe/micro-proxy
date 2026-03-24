package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/config"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/app"
	"github.com/nunoOliveiraqwe/micro-proxy/middleware"
	"go.uber.org/zap"
)

func StartServer(conf config.APIServerConfig, systemService app.SystemService) *http.Server {
	zap.S().Infof("starting local server at %s:%d", conf.Host, conf.Port)

	if conf.Port <= 0 || conf.Port > 65535 {
		zap.S().Fatalf("invalid port number %d", conf.Port)
		return nil
	}

	httpServer := &http.Server{
		Addr: fmt.Sprintf("%s:%d", conf.Host, conf.Port),
	}
	httpServer.IdleTimeout = time.Duration(conf.IdleTimeoutSecs) * time.Second
	httpServer.ReadTimeout = time.Duration(conf.ReadTimeoutSecs) * time.Second
	httpServer.WriteTimeout = time.Duration(conf.WriteTimeoutSecs) * time.Second

	mux := buildMux(conf.Port, systemService)
	httpServer.Handler = systemService.SessionRegistry().WrapWithSessionMiddleware(mux)
	return httpServer

}

func buildMux(port int, svc app.SystemService) *http.ServeMux {
	zap.S().Debugf("Building http mux for proxy API")
	mux := http.NewServeMux()
	for _, route := range routes {
		zap.S().Debugf("Initializing route named %s with path %s", route.Name, route.Pattern)
		fullPathWithMethod := fmt.Sprintf("%s %s%s", route.Method, APPLICATION_ROUTE_BASE_PATH, route.Pattern)
		zap.S().Debugf("Full path for route %s is %s", route.Name, fullPathWithMethod)
		routeHandlerFunc := route.HandlerFunc(svc)
		routeHandlerFunc = middleware.RequestIDMiddleware(routeHandlerFunc, middleware.Config{})
		routeHandlerFunc = middleware.RequestLoggerMiddleware(routeHandlerFunc, middleware.Config{})

		confMetrics := middleware.Config{
			Type:    "Metrics",
			Options: make(map[string]interface{}),
		}
		confMetrics.Options[middleware.MgrKey] = svc.GetGlobalMetricsManager()
		confMetrics.Options["port"] = strconv.Itoa(port)
		routeHandlerFunc = middleware.MetricsMiddleware(routeHandlerFunc, confMetrics)
		routeHandlerFunc = checkIfRouteIsAllowedIfFtsIsNotDone(routeHandlerFunc, route.IsAllowedBeforeFts,
			route.IsAllowedAfterFts, svc)
		if route.IsSecure {
			routeHandlerFunc = isAuthenticatedRequest(routeHandlerFunc, svc)
		}
		mux.HandleFunc(fullPathWithMethod, routeHandlerFunc)
		zap.S().Debugf("Route %s initialized", route.Name)
	}

	zap.S().Debugf("Registering UI routes")
	registerUIRoutes(mux, svc)

	return mux
}
