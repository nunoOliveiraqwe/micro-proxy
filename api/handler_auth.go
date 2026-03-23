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
		err = systemService.GetServiceStore().GetUserService().PasswordMatchesForUser(l.Password, l.Username)
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

func handleLogout(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Debugf("Handling logout request")
		systemService.SessionRegistry().LogoutSession(w, r)
		zap.S().Infof("Logout successful")
	}
}

func handleChangePassword(service app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Debugf("Handling change password request")
		c, err := DecodeJSONBody[ChangePasswordRequest](r)
		if err != nil {
			zap.S().Errorf("Failed to decode change password request: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		username := service.SessionRegistry().GetValueFromSession(r, "username")
		if username == "" {
			zap.S().Errorf("No valid session found for change password request")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		err = service.GetServiceStore().GetUserService().PasswordMatchesForUser(c.OldPassword, username)
		if err != nil {
			zap.S().Errorf("Old password verification failed for user %s: %v", username, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		err = service.GetServiceStore().GetUserService().SetPasswordForUser(c.NewPassword, username)
		if err != nil {
			zap.S().Errorf("Failed to change password for user %s: %v", username, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
}

func handleIdentity(service app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Debugf("Handling identity request")
		username := service.SessionRegistry().GetValueFromSession(r, "username")
		if username == "" {
			zap.S().Errorf("No valid session found for change password request")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		zap.S().Infof("User %s is authenticated", username)
		ident := UserIdentityResponse{Username: username}
		WriteResponseAsJSON(ident, w)
	}
}
