package api

import (
	"errors"
	"net/http"
	"sync"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/service"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

// ftsLock serialises first-time-setup completion so that concurrent
// requests cannot both pass the "is FTS done?" guard and race to set
// the admin password.
var ftsLock sync.Mutex

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

		ftsLock.Lock()
		defer ftsLock.Unlock()

		// Re-check under the lock: another request may have completed FTS
		// while this one was waiting.
		if systemService.GetServiceStore().GetSystemConfigurationService().IsFirstTimeSetupCompleted() {
			logger.Warn("FTS already completed, rejecting duplicate request")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		logger.Info("Received FTS completion request, setting admin user password")
		err = systemService.GetServiceStore().GetUserService().
			SetPasswordForUser(f.Password, "admin")
		if err != nil {
			var pve *service.PasswordValidationError
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
		logger.Info("FTS completed successfully")
	}
}
