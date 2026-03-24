package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/app"
)

func handleGetSystemHealth(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteResponseAsJSON(svc.GetSystemHealth(), w)
	}
}

func handleGetRecentErrors(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteResponseAsJSON(svc.GetRecentErrors(50), w)
	}
}

func handleGetRecentRequests(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteResponseAsJSON(svc.GetRecentRequests(200), w)
	}
}
