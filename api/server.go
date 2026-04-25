package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
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

	mux := buildMux(conf.Port, conf.AccessLogPath, systemService)
	httpServer.Handler = systemService.SessionRegistry().WrapWithSessionMiddleware(mux)
	return httpServer

}

func buildMux(port int, logPath string, svc app.SystemService) *http.ServeMux {
	zap.S().Debugf("Building http mux for proxy API")
	mux := http.NewServeMux()

	ctx := context.WithValue(context.Background(), ctxkeys.Port, strconv.Itoa(port))
	ctx = context.WithValue(ctx, ctxkeys.MetricsMgr, svc.GetGlobalMetricsManager())
	ctx = context.WithValue(ctx, ctxkeys.ServerID, fmt.Sprintf("http-%d", port))
	ctx = context.WithValue(ctx, ctxkeys.CacheInsightMgr, svc.GetCacheInsightManager())

	globalHandler := mux.ServeHTTP

	globalHandler = middleware.MetricsMiddleware(ctx, globalHandler, middleware.Config{})
	globalHandler = middleware.BodySizeLimitMiddleware(ctx, globalHandler, middleware.Config{
		Options: map[string]interface{}{"max-size": "20m"},
	})
	globalHandler = middleware.RequestLoggerMiddleware(ctx, globalHandler, middleware.Config{
		Options: map[string]interface{}{"request-log-path": logPath}})
	globalHandler = middleware.RequestIDMiddleware(ctx, globalHandler, middleware.Config{})

	for _, route := range routes {
		zap.S().Debugf("Initializing route named %s with path %s", route.Name, route.Pattern)
		fullPathWithMethod := fmt.Sprintf("%s %s%s", route.Method, APPLICATION_ROUTE_BASE_PATH, route.Pattern)
		zap.S().Debugf("Full path for route %s is %s", route.Name, fullPathWithMethod)

		routeHandlerFunc := route.HandlerFunc(svc)

		if route.IsSecure {
			if len(route.KeyAuth.Scopes) > 0 {
				routeHandlerFunc = isAuthenticatedBySessionOrApiKey(routeHandlerFunc, route.KeyAuth.Scopes, svc)
			} else {
				routeHandlerFunc = isAuthenticatedRequest(routeHandlerFunc, svc)
			}
		}
		routeHandlerFunc = checkIfRouteIsAllowedIfFtsIsNotDone(routeHandlerFunc, route.IsAllowedBeforeFts, route.IsAllowedAfterFts, svc)
		if route.IsRateLimited {
			limiterReq := map[string]interface{}{
				"rate-per-second": 10.0,
				"burst":           15,
			}
			optMap := map[string]interface{}{
				"mode":        "per-client",
				"limiter-req": limiterReq,
				"cache-ttl":   "72h", //we really want the limiter to not refresh/be recreated,
				// however, for clients that connect, send a shit burst, then disconnect, we effectively hold up memory for assholes
				// and wasting resources for assholes is kinda bad, so like everything in life, we compromise
				"cleanup-interval": "6h",
				"max-cache-size":   100000,
			}
			conf := middleware.Config{
				Options: optMap,
			}
			routeHandlerFunc = middleware.RateLimitMiddleware(ctx, routeHandlerFunc, conf)
		}
		mux.HandleFunc(fullPathWithMethod, routeHandlerFunc)
		zap.S().Debugf("Route %s initialized", route.Name)
	}

	zap.S().Debugf("Registering UI routes")
	registerUIRoutes(mux, svc)

	wrappedMux := http.NewServeMux()
	wrappedMux.HandleFunc("/", globalHandler)
	return wrappedMux
}
