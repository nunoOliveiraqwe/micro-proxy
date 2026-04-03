package api

import (
	"errors"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func handleGetFtsStatus(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Handling FTS status request")
		isFtsCompleted := systemService.GetServiceStore().GetSystemConfigurationService().
			IsFirstTimeSetupCompleted()
		logger.Info("FTS status", zap.Bool("completed", isFtsCompleted))
		respDto := FtsStatusResponse{
			IsFtsCompleted: isFtsCompleted,
		}
		WriteResponseAsJSON(respDto, w)
	}
}

func handleCompleteFts(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Info("Handling FTS completion request")
		f, err := DecodeJSONBody[CompleteFtsRequest](r)
		if err != nil {
			logger.Error("Failed to decode FTS completion request", zap.Error(err))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		logger.Info("Received FTS completion request, setting admin user password")
		err = systemService.GetServiceStore().GetUserService().
			SetPasswordForUser(f.Password, "admin")
		if err != nil {
			var pve *app.PasswordValidationError
			if errors.As(err, &pve) {
				logger.Error("Invalid password provided for FTS completion", zap.Error(err))
				http.Error(w, "Invalid password: "+pve.Error(), http.StatusBadRequest)
				return
			}
			logger.Error("Failed to set admin user password", zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		logger.Info("Admin user password set successfully")
		err = systemService.GetServiceStore().
			GetSystemConfigurationService().
			CompleteFistTimeSetup()
		if err != nil {
			logger.Error("Failed to complete FTS", zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		handleLogin(systemService).ServeHTTP(w, r)
		logger.Info("FTS completed successfully")
	}
}
