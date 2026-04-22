package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/auth"
	"go.uber.org/zap"
)

type basicAuth struct {
	realm       string
	credentials map[string]string
	encoder     auth.Encoder
}

func BasicAuthMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	b, err := parseBasicAuthOptions(conf)
	if err != nil {
		zap.S().Errorf("BasicAuthMiddleware: failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "BasicAuthMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q`, b.realm))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		logger.Debug("BasicAuthMiddleware: received credentials", zap.String("username", username))
		hash, found := b.credentials[username]
		if !found {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err := b.encoder.Matches(password, hash); err != nil {
			logger.Debug("BasicAuthMiddleware: invalid password", zap.String("username", username), zap.Error(err))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func parseBasicAuthOptions(conf Config) (*basicAuth, error) {
	zap.S().Debug("Parsing basic auth options")

	realm, err := ParseStringRequired(conf.Options, "realm")
	if err != nil {
		return nil, err
	}

	rawCreds, ok := conf.Options["credentials"]
	if !ok || rawCreds == nil {
		return nil, fmt.Errorf("credentials is required")
	}

	credentials := make(map[string]string)

	switch typed := rawCreds.(type) {
	case map[string]interface{}:
		for username, v := range typed {
			password, _ := v.(string)
			if username == "" || password == "" {
				return nil, fmt.Errorf("credentials must have non-empty username and password")
			}
			credentials[username] = password
		}
	case map[interface{}]interface{}:
		for k, v := range typed {
			username, _ := k.(string)
			password, _ := v.(string)
			if username == "" || password == "" {
				return nil, fmt.Errorf("credentials must have non-empty username and password")
			}
			credentials[username] = password
		}
	default:
		return nil, fmt.Errorf("credentials must be a map of username to password hash, got %T", rawCreds)
	}

	if len(credentials) == 0 {
		return nil, fmt.Errorf("credentials must not be empty")
	}

	return &basicAuth{
		realm:       realm,
		credentials: credentials,
		encoder:     auth.NewDefaultEncoder(),
	}, nil
}

/**
routes:
  - host: internal.example.com
    middlewares:
      - type: basic_auth
        realm: "Internal"
        credentials:
          nuno: "$argon2id$v=19$..."
          alice: "$argon2id$v=19$..."
*/
