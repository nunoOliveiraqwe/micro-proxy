package api

import (
	"net/http"
	"strconv"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func handleGetProxies(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(request)
		logger.Debug("Fetching configured proxy servers")
		proxies := systemService.GetConfiguredProxyServers()
		if proxies == nil {
			logger.Error("Failed to retrieve configured proxy servers")
			http.Error(writer, "Failed to retrieve configured proxy servers", http.StatusInternalServerError)
			return
		}
		logger.Debug("Retrieved configured proxy servers", zap.Int("count", len(proxies)))
		WriteResponseAsJSON(proxies, writer)
	}
}

func handleStartProxy(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(request)
		port := request.PathValue("serverId")
		logger.Info("Starting proxy server", zap.String("port", port))
		portInt, err := strconv.Atoi(port)
		if err != nil {
			logger.Error("Invalid port format", zap.String("port", port))
			http.Error(writer, "Invalid port format", http.StatusBadRequest)
			return
		}
		err = systemService.StartProxy(portInt)
		if err != nil {
			logger.Error("Failed to start proxy server", zap.String("port", port), zap.Error(err))
			http.Error(writer, "Failed to start proxy server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		WriteResponseAsJSON(map[string]string{"status": "started"}, writer)
	}
}

func handleStopProxy(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(request)
		port := request.PathValue("serverId")
		logger.Info("Stopping proxy server", zap.String("port", port))
		portInt, err := strconv.Atoi(port)
		if err != nil {
			logger.Error("Invalid port format", zap.String("port", port))
			http.Error(writer, "Invalid port format", http.StatusBadRequest)
			return
		}
		err = systemService.StopProxy(portInt)
		if err != nil {
			logger.Error("Failed to stop proxy server", zap.String("port", port), zap.Error(err))
			http.Error(writer, "Failed to stop proxy server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		WriteResponseAsJSON(map[string]string{"status": "stopped"}, writer)
	}
}

func handleGetGlobalMetrics(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(request)
		logger.Info("Fetching global proxy metrics")
		globalMetrics := systemService.GetGlobalMetricsManager().GetGlobalMetrics()
		WriteResponseAsJSON(globalMetrics, writer)
	}
}

func handleGetMetricForConnection(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(request)
		serverId := request.PathValue("serverId")
		logger.Info("Fetching metric for connection", zap.String("serverId", serverId))
		metrics := systemService.GetGlobalMetricsManager().GetAllMetricsByServer(serverId)
		WriteResponseAsJSON(metrics, writer)
	}
}
