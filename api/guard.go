package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
)

func checkIfRouteIsAllowedIfFtsIsNotDone(next http.HandlerFunc, isAllowedBeforeFts, isAllowedAfterFts bool, svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Checking if route is allowed when FTS is not done")
		if isAllowedAfterFts && isAllowedBeforeFts {
			next(w, r)
			return
		}
		isFtsDone := svc.GetServiceStore().GetSystemConfigurationService().IsFirstTimeSetupCompleted()
		if isAllowedAfterFts && isFtsDone {
			logger.Debug("Route is allowed because FTS is done")
			next(w, r)
		} else if isAllowedBeforeFts && !isFtsDone {
			logger.Debug("Route is allowed because FTS is not done")
			next(w, r)
		} else {
			logger.Debug("Route is not allowed because FTS is not done")
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
	}
}

func isAuthenticatedRequest(next http.HandlerFunc, systemService app.SystemService) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(request)
		logger.Debug("Checking if request is authenticated")
		isValid := systemService.SessionRegistry().HasValidSession(request)
		if !isValid {
			logger.Debug("Request is not authenticated")
			http.Error(writer, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(writer, request)
	}
}
