package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
)

func handleGetSystemHealth(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching system health")
		WriteResponseAsJSON(svc.GetSystemHealth(), w)
	}
}

func handleGetRecentErrors(svc app.SystemService) http.HandlerFunc {
	errCap, _, _ := svc.GetGlobalMetricsManager().GetLogCapacities()
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching recent error logs")
		WriteResponseAsJSON(svc.GetRecentErrors(errCap), w)
	}
}

func handleGetRecentRequests(svc app.SystemService) http.HandlerFunc {
	_, reqCap, _ := svc.GetGlobalMetricsManager().GetLogCapacities()
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching recent request logs")
		WriteResponseAsJSON(svc.GetRecentRequests(reqCap), w)
	}
}

func handleGetRecentBlocked(svc app.SystemService) http.HandlerFunc {
	_, _, blkCap := svc.GetGlobalMetricsManager().GetLogCapacities()
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching recent blocked entries")
		WriteResponseAsJSON(svc.GetRecentBlockedEntries(blkCap), w)
	}
}
