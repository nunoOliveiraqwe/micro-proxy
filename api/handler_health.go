package api

import (
	"net/http"

	"github.com/nunoOliveiraqwe/torii/internal/app"
)

func handleHealthCheck(_ app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}
