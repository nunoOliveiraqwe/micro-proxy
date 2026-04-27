package api

import (
	"errors"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/service"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func handleCreateNewApiKey(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Info("Handling create new API key request")
		apiKeyRequest, err := DecodeJSONBody[service.CreateApiKeyRequest](r)
		if err != nil {
			logger.Warn("Failed to decode create API key request", zap.Error(err))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		apiKey, err := systemService.GetServiceStore().
			GetApiKeyService().CreateApiKey(r.Context(), apiKeyRequest)
		if err != nil {
			if errors.Is(err, service.ErrorInvalidApiKeyRequest) {
				logger.Warn("Invalid API key request", zap.Error(err))
				http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
				return
			} else if errors.Is(err, service.ErrorDuplicatedAliasApiKeyRequest) {
				logger.Warn("API key already exists", zap.Error(err))
				http.Error(w, "Conflict: "+err.Error(), http.StatusConflict)
				return
			} else if errors.Is(err, service.ErrorInvalidScopesApiKeyRequest) {
				logger.Warn("Invalid API key scopes", zap.Error(err))
				http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
				return
			} else if errors.Is(err, service.ErrorInvalidAliasApiKeyRequest) {
				logger.Warn("Invalid API key expiration", zap.Error(err))
				http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
				return
			} else if errors.Is(err, service.ErrorInvalidExpiryDateApiKeyRequest) {
				logger.Warn("Invalid API key expiration", zap.Error(err))
				http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
				return
			}
			logger.Error("Failed to create API key", zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		WriteResponseAsJSON(apiKey, w)
	}
}

func handleGetAllApiKey(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Info("Handling get all API keys request")
		apiKeys := systemService.GetServiceStore().GetApiKeyService().GetAllApiKeys(r.Context())
		WriteResponseAsJSON(apiKeys, w)
	}
}

func handleDeleteApiKey(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		alias := r.PathValue("id")
		if alias == "" {
			logger.Warn("Missing API key alias in delete request")
			http.Error(w, "Bad request: missing API key alias", http.StatusBadRequest)
			return
		}
		logger.Info("Handling delete API key request", zap.String("alias", alias))
		err := systemService.GetServiceStore().GetApiKeyService().DeleteApiKey(r.Context(), alias)
		if err != nil {
			logger.Error("Failed to delete API key", zap.String("alias", alias), zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
