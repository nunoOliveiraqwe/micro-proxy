package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/micro-proxy/internal/app"
	"go.uber.org/zap"
)

func handleLogin(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Debugf("Handling login request")
		l, err := DecodeJSONBody[LoginRequest](r)
		if err != nil {
			zap.S().Errorf("Failed to decode login request: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized) //we give no INFO
			return
		}
		zap.S().Infof("Authenticating user %s", l.Username)
		err = systemService.GetServiceStore().GetUserService().PasswordMatches(l.Password, l.Username)
		if err != nil {
			zap.S().Errorf("Password verification failed for user %s: %v", l.Username, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		err = systemService.SessionRegistry().NewSession(r, w, l.Username)
		if err != nil {
			zap.S().Errorf("Failed to create session for user %s: %v", l.Username, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		zap.S().Infof("Login successful for user %s", l.Username)
	}
}
