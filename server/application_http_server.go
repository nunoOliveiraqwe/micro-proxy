package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/configuration"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/manager"
	"github.com/nunoOliveiraqwe/micro-proxy/middleware"
	"go.uber.org/zap"
)

func StartServer(conf configuration.APIServerConfig, systemManager manager.SystemManager) *http.Server {
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

	httpServer.Handler = buildMux(systemManager)
	return httpServer

}

func buildMux(manager manager.SystemManager) *http.ServeMux {
	zap.S().Debugf("Building http mux for proxy API")
	mux := http.NewServeMux()
	for _, route := range routes {
		zap.S().Debugf("Initializing route named %s with path %s", route.Name, route.Pattern)
		fullPathWithMethod := fmt.Sprintf("%s %s%s", route.Method, APPLICATION_ROUTE_BASE_PATH, route.Pattern)
		zap.S().Debugf("Full path for route %s is %s", route.Name, fullPathWithMethod)
		routeHandlerFunc := route.HandlerFunc(manager)
		routeHandlerFunc = middleware.RequestIDMiddleware(routeHandlerFunc, middleware.MiddlewareConfiguration{})
		routeHandlerFunc = middleware.RequestLoggerMiddleware(routeHandlerFunc, middleware.MiddlewareConfiguration{})
		mux.HandleFunc(fullPathWithMethod, routeHandlerFunc)
		zap.S().Debugf("Route %s initialized", route.Name)
	}
	return mux
}
