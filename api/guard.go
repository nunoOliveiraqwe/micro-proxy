package api

import (
	"net/http"
	"strings"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func checkIfRouteIsAllowedIfFtsIsNotDone(next http.HandlerFunc, isAllowedBeforeFts, isAllowedAfterFts bool, svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		if isAllowedAfterFts && isAllowedBeforeFts {
			next(w, r)
			return
		}
		isFtsDone := svc.GetServiceStore().GetSystemConfigurationService().IsFirstTimeSetupCompleted()
		if isAllowedAfterFts && isFtsDone {
			next(w, r)
		} else if isAllowedBeforeFts && !isFtsDone {
			next(w, r)
		} else {
			logger.Warn("Route is not allowed because FTS is not done")
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

// isAuthenticatedBySessionOrApiKey allows access if the request has a valid
// session OR carries a valid API key with at least one of the required scopes.
// The API key is read from the Authorization header: "Bearer <key>".
func isAuthenticatedBySessionOrApiKey(next http.HandlerFunc, requiredScopes []domain.Scope, svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)

		if svc.SessionRegistry().HasValidSession(r) {
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			logger.Debug("No session and no Authorization header")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			logger.Debug("Authorization header is not Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		rawKey := strings.TrimPrefix(authHeader, bearerPrefix)
		if rawKey == "" {
			logger.Debug("Bearer token is empty")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		apiKeySvc := svc.GetServiceStore().GetApiKeyService()
		for _, scope := range requiredScopes {
			valid, err := apiKeySvc.IsKeyValidForScope(rawKey, string(scope))
			if err != nil {
				logger.Error("Error validating API key scope", zap.Error(err))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if valid {
				logger.Debug("API key authorized", zap.String("scope", string(scope)))
				next(w, r)
				return
			}
		}

		logger.Debug("API key does not have required scopes")
		http.Error(w, "Forbidden", http.StatusForbidden)
	}
}
