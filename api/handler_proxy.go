package api

import (
	"net/http"
	"strconv"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/app"
	"github.com/nunoOliveiraqwe/micro-proxy/metrics"
	"go.uber.org/zap"
)

func handleGetProxies(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		zap.S().Infof("Fetching configured proxy servers")
		proxies := systemService.GetConfiguredProxyServers()
		if proxies == nil {
			zap.S().Errorf("Failed to retrieve configured proxy servers")
			http.Error(writer, "Failed to retrieve configured proxy servers", http.StatusInternalServerError)
			return
		}
		zap.S().Infof("Retrieved %d configured proxy servers", len(proxies))
		WriteResponseAsJSON(proxies, writer)
	}
}

func handleStartProxy(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		port := request.PathValue("connectionId")
		zap.S().Infof("Starting proxy server on port %s", port)
		portInt, err := strconv.Atoi(port)
		if err != nil {
			zap.S().Errorf("Invalid port format: %s", port)
			http.Error(writer, "Invalid port format", http.StatusBadRequest)
			return
		}
		err = systemService.StartProxy(portInt)
		if err != nil {
			zap.S().Errorf("Failed to start proxy server on port %s: %v", port, err)
			http.Error(writer, "Failed to start proxy server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		WriteResponseAsJSON(map[string]string{"status": "started"}, writer)
	}
}

func handleStopProxy(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		port := request.PathValue("connectionId")
		zap.S().Infof("Stopping proxy server on port %s", port)
		portInt, err := strconv.Atoi(port)
		if err != nil {
			zap.S().Errorf("Invalid port format: %s", port)
			http.Error(writer, "Invalid port format", http.StatusBadRequest)
			return
		}
		err = systemService.StopProxy(portInt)
		if err != nil {
			zap.S().Errorf("Failed to stop proxy server on port %s: %v", port, err)
			http.Error(writer, "Failed to stop proxy server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		WriteResponseAsJSON(map[string]string{"status": "stopped"}, writer)
	}
}

func handleGetGlobalMetrics(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		zap.S().Infof("Fetching global proxy metrics")
		globalMetrics := systemService.GetGlobalMetricsManager().GetGlobalMetrics()
		WriteResponseAsJSON(globalMetrics, writer)
	}
}

func handleGetMetricForConnection(systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		connectionId := request.PathValue("connectionId")
		zap.S().Infof("Fetching metric for connection %s", connectionId)
		metric := systemService.GetGlobalMetricsManager().GetMetricForConnection(connectionId)
		if metric == nil {
			metric = &metrics.Metric{}
		}
		WriteResponseAsJSON(metric, writer)
	}
}
