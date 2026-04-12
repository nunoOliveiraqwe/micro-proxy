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
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching recent error logs")
		WriteResponseAsJSON(svc.GetRecentErrors(50), w)
	}
}

func handleGetRecentRequests(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching recent request logs")
		WriteResponseAsJSON(svc.GetRecentRequests(200), w)
	}
}

func handleGetRecentBlocked(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching recent blocked entries")
		WriteResponseAsJSON(svc.GetRecentBlockedEntries(50), w)
	}
}

