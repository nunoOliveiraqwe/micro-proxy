package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/app"
	"go.uber.org/zap"
)

func handleGetFtsStatus(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Debugf("Handling FTS status request")
		isFtsCompleted := systemService.GetServiceStore().GetSystemConfigurationService().
			IsFirstTimeSetupCompleted()
		zap.S().Infof("FTS status: %v", isFtsCompleted)
		respDto := FtsStatusResponse{
			IsFtsCompleted: isFtsCompleted,
		}
		WriteResponseAsJSON(respDto, w)
	}
}

func handleCompleteFts(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Infof("Handling FTS completion request")
		f, err := DecodeJSONBody[CompleteFtsRequest](r)
		if err != nil {
			zap.S().Errorf("Failed to decode FTS completion request: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		zap.S().Infof("Received FTS completion request, setting admin user password")
		err = systemService.GetServiceStore().GetUserService().
			SetPasswordForUser("admin", f.Password)
		if err != nil {
			zap.S().Errorf("Failed to set admin user password: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		zap.S().Infof("Admin user password set successfully")
		err = systemService.GetServiceStore().
			GetSystemConfigurationService().
			CompleteFistTimeSetup()
		if err != nil {
			zap.S().Errorf("Failed to complete FTS: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		handleLogin(systemService)(w, r)
		zap.S().Infof("FTS completed successfully")
	}
}
