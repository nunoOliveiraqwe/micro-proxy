package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

func handleLogin(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Handling login request")
		l, err := DecodeJSONBody[LoginRequest](r)
		if err != nil {
			logger.Error("Failed to decode login request", zap.Error(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized) //we give no INFO
			return
		}
		logger.Info("Authenticating user", zap.String("username", l.Username))
		err = systemService.GetServiceStore().GetUserService().PasswordMatchesForUser(l.Password, l.Username)
		if err != nil {
			logger.Error("Password verification failed", zap.String("username", l.Username), zap.Error(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		err = systemService.SessionRegistry().NewSession(r, w, l.Username)
		if err != nil {
			logger.Error("Failed to create session", zap.String("username", l.Username), zap.Error(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		logger.Info("Login successful", zap.String("username", l.Username))
	}
}

func handleLogout(systemService app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Handling logout request")
		systemService.SessionRegistry().LogoutSession(w, r)
		logger.Info("Logout successful")
	}
}

func handleChangePassword(service app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Handling change password request")
		c, err := DecodeJSONBody[ChangePasswordRequest](r)
		if err != nil {
			logger.Error("Failed to decode change password request", zap.Error(err))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		username := service.SessionRegistry().GetValueFromSession(r, "username")
		if username == "" {
			logger.Error("No valid session found for change password request")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		err = service.GetServiceStore().GetUserService().PasswordMatchesForUser(c.OldPassword, username)
		if err != nil {
			logger.Error("Old password verification failed", zap.String("username", username), zap.Error(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		err = service.GetServiceStore().GetUserService().SetPasswordForUser(c.NewPassword, username)
		if err != nil {
			logger.Error("Failed to change password", zap.String("username", username), zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
}

func handleIdentity(service app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Handling identity request")
		username := service.SessionRegistry().GetValueFromSession(r, "username")
		if username == "" {
			logger.Error("No valid session found for identity request")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		logger.Info("User is authenticated", zap.String("username", username))
		ident := UserIdentityResponse{Username: username}
		WriteResponseAsJSON(ident, w)
	}
}
