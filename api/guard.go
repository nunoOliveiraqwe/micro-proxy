package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/app"
	"github.com/nunoOliveiraqwe/micro-proxy/middleware"
	"go.uber.org/zap"
)

func checkIfRouteIsAllowedIfFtsIsNotDone(next http.HandlerFunc, isAllowedBeforeFts, isAllowedAfterFts bool, svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Debugf("Checking if route %s is allowed when FTS is not done", r.URL.Path)
		if isAllowedAfterFts && isAllowedBeforeFts {
			next(w, r)
			return
		}
		isFtsDone := svc.GetServiceStore().GetSystemConfigurationService().IsFirstTimeSetupCompleted()
		if isAllowedAfterFts && isFtsDone {
			zap.S().Debugf("Route %s is allowed because FTS is done", r.URL.Path)
			next(w, r)
		} else if isAllowedBeforeFts && !isFtsDone {
			zap.S().Debugf("Route %s is allowed because FTS is not done", r.URL.Path)
			next(w, r)
		} else {
			zap.S().Debugf("Route %s is not allowed because FTS is not done", r.URL.Path)
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
	}
}

func isAuthenticatedRequest(next http.HandlerFunc, systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		reqId := middleware.GetRequestIDFromContext(request.Context())
		zap.S().Debugf("Checking if request %s is authenticated", reqId)
		isValid := systemService.SessionRegistry().HasValidSession(request)
		if !isValid {
			zap.S().Debugf("Request %s is not authenticated", reqId)
			http.Error(writer, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(writer, request)
	}
}
