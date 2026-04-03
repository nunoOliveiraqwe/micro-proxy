package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/nunoOliveiraqwe/torii/config"
	"go.uber.org/zap"
)

type PathDispatcher struct {
	mux *http.ServeMux
}

func (d *PathDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mux.ServeHTTP(w, r)
}

func buildPathDispatcher(ctx context.Context, defaultHandler http.HandlerFunc, pathRules []config.PathRule, baseProxy http.HandlerFunc) (http.Handler, []string, error) {
	mux := http.NewServeMux()

	var mwNames []string

	for _, rule := range pathRules {
		pattern := normalizePattern(rule.Pattern)
		ctx2 := context.WithValue(ctx, "path", rule.Pattern)
		handler, err := buildMiddlewareChain(ctx2, baseProxy, rule.Middlewares)
		if err != nil {
			return nil, nil, err
		}
		mux.HandleFunc(pattern, handler)
		mwNames = append(mwNames, middlewareNames(rule.Middlewares)...)
		zap.S().Infof("Registered path rule %q with %d middlewares", pattern, len(rule.Middlewares))
	}

	// Default catch-all: the route-level middleware chain wrapping the backend.
	mux.HandleFunc("/", defaultHandler)

	return &PathDispatcher{mux: mux}, mwNames, nil
}

// normalizePattern converts user-friendly glob patterns into Go ServeMux
// patterns. A trailing /* becomes a catch-all wildcard, and any mid-path *
// becomes a single-segment wildcard.
//
// Examples:
//
//	/api/v1/users/*       → /api/v1/users/{path...}   (catch-all)
//	/users/*/start        → /users/{_seg1}/start       (single-segment wildcard)
//	/users/*/jobs/*       → /users/{_seg1}/jobs/{path...}
//	/api/v1/users/        → /api/v1/users/             (prefix match, unchanged)
//	/health               → /health                    (exact match, unchanged)
//	/users/whatever/stop  → /users/whatever/stop        (concrete, unchanged)
//
// Precedence in Go's ServeMux: concrete segments always beat wildcards, so
// /users/whatever/stop wins over /users/{_seg1}/stop for that specific path.
func normalizePattern(pattern string) string {
	segments := strings.Split(pattern, "/")
	wildIdx := 0
	for i, seg := range segments {
		if seg != "*" {
			continue
		}
		if i == len(segments)-1 {
			// Trailing /* → catch-all
			segments[i] = "{path...}"
		} else {
			// Mid-path * → single-segment named wildcard
			wildIdx++
			segments[i] = fmt.Sprintf("{_seg%d}", wildIdx)
		}
	}
	return strings.Join(segments, "/")
}
