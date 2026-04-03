package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
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

		ctx := context.WithValue(context.Background(), "port", strconv.Itoa(port))
		//ctx = context.WithValue(ctx, "path", route.Pattern)
		ctx = context.WithValue(ctx, middleware.MgrKey, svc.GetGlobalMetricsManager())
		ctx = context.WithValue(ctx, "serverId", fmt.Sprintf("http-%d", port))
		if route.IsSecure {
			routeHandlerFunc = isAuthenticatedRequest(routeHandlerFunc, svc)
		}
		routeHandlerFunc = checkIfRouteIsAllowedIfFtsIsNotDone(routeHandlerFunc, route.IsAllowedBeforeFts, route.IsAllowedAfterFts, svc)
		routeHandlerFunc = middleware.MetricsMiddleware(ctx, routeHandlerFunc, middleware.Config{})
		routeHandlerFunc = middleware.RequestLoggerMiddleware(ctx, routeHandlerFunc, middleware.Config{})
		routeHandlerFunc = middleware.RequestIDMiddleware(ctx, routeHandlerFunc, middleware.Config{})
		mux.HandleFunc(fullPathWithMethod, routeHandlerFunc)
		zap.S().Debugf("Route %s initialized", route.Name)
	}

	zap.S().Debugf("Registering UI routes")
	registerUIRoutes(mux, svc)

	return mux
}
