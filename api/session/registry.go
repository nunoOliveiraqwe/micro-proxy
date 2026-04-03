package session

import (
	"context"
	"net/http"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"go.uber.org/zap"
)

const cookieName = "torii-id"
const cookiePath = "/"

type Registry struct {
	manager *scs.SessionManager
}

func NewRegistry(db *sqlite.DB, cfg config.SessionConfig) *Registry {
	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.NewWithCleanupInterval(db.GetDb(), cfg.CleanupInterval)
	sessionManager.Lifetime = cfg.Lifetime
	sessionManager.IdleTimeout = cfg.IdleTimeout
	sessionManager.Cookie.Name = cookieName
	sessionManager.Cookie.Domain = cfg.CookieDomain
	sessionManager.Cookie.HttpOnly = cfg.CookieHttpOnly
	sessionManager.Cookie.Path = cookiePath
	sessionManager.Cookie.Persist = true
	sessionManager.Cookie.SameSite = cfg.SameSiteMode()
	sessionManager.Cookie.Secure = cfg.CookieSecure

	return &Registry{manager: sessionManager}
}

func (reg *Registry) RenewSession(r *http.Request) error {
	err := reg.manager.RenewToken(r.Context())
	if err != nil {
		zap.L().Error("failed to renew session", zap.Error(err))
		return err
	}
	return nil
}

func (reg *Registry) GetValueFromSession(r *http.Request, key string) string {
	zap.S().Debugf("Fetching value with key %s from session", key)
	return reg.manager.GetString(r.Context(), key)
}

func (reg *Registry) NewSession(r *http.Request, w http.ResponseWriter, username string) error {
	zap.S().Infof("Creating new session for user %s", username)
	err := reg.RenewSession(r)
	if err != nil {
		return err
	}
	reg.manager.Put(r.Context(), "username", username)
	return nil
}

func (reg *Registry) HasValidSession(r *http.Request) bool {
	token := reg.manager.Token(r.Context())
	return token != ""
}

func (reg *Registry) LogoutSession(w http.ResponseWriter, r *http.Request) {
	reg.manager.Remove(r.Context(), "username")
	err := reg.manager.Destroy(r.Context())
	if err != nil {
		zap.S().Errorf("Cannot logout session, %v", err)
	}
}

func (reg *Registry) WrapWithSessionMiddleware(next http.Handler) http.Handler {
	return reg.manager.LoadAndSave(next)
}

func (reg *Registry) KillSessionsForUser(username string) {
	contexts := []context.Context{}
	err := reg.manager.Iterate(context.Background(), func(ctx context.Context) error {
		name := reg.manager.GetString(ctx, "username")
		if name == username {
			zap.S().Debugf("Appending context to terminate list")
			contexts = append(contexts, ctx)
		}
		return nil
	})
	if err != nil {
		zap.S().Errorf("Cannot kill sessions for user %s, %v", username, err)
		return
	}
	for _, ctx := range contexts {
		err := reg.manager.Destroy(ctx)
		if err != nil {
			zap.S().Errorf("Cannot logout session, %v", err)
		}
	}
}
