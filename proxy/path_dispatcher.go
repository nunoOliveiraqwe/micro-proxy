package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/proxyutil"
	"go.uber.org/zap"
)

type PathDispatcher struct {
	mux *http.ServeMux
}

func (d *PathDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mux.ServeHTTP(w, r)
}

func buildPathDispatcher(ctx context.Context, defaultHandler http.HandlerFunc, pathRules []config.PathRule) (http.Handler, []string, []string, error) {
	mux := http.NewServeMux()

	mwNames := make([]string, 0)
	backends := make([]string, 0)
	hasCatchAll := false
	for _, rule := range pathRules {
		pathBaseHandler := defaultHandler
		if rule.Backend != "" {
			zap.S().Infof("Building backend handler for path rule %q with backend %q", rule.Pattern, rule.Backend)
			handler, err := buildPathBackendHandler(rule)
			if err != nil {
				return nil, nil, nil, err
			}
			pathBaseHandler = handler
			backends = append(backends, rule.Backend)
		}

		pattern := normalizePattern(rule.Pattern)
		ctx2 := context.WithValue(ctx, ctxkeys.Path, rule.Pattern)
		handler, err := buildMiddlewareChain(ctx2, pathBaseHandler, rule.Middlewares, rule.DisableDefaults)
		if err != nil {
			return nil, nil, nil, err
		}

		mux.HandleFunc(pattern, handler)

		if pattern == "/" || pattern == "/{path...}" {
			hasCatchAll = true
		}
		// When the rule has its own backend and the pattern is a bare path
		// (e.g. "/jellyfino"), Go's ServeMux treats it as an exact match
		// only.  We also need a catch-all so that sub-paths like
		// "/jellyfino/library" are routed to this backend instead of
		// falling through to the default handler.
		if rule.Backend != "" && !strings.HasSuffix(pattern, "/") && !strings.Contains(pattern, "{path...}") {
			mux.HandleFunc(pattern+"/{path...}", handler)
		}

		mwNames = append(mwNames, middlewareNames(rule.Middlewares)...)
		zap.S().Infof("Registered path rule %q with %d middlewares", pattern, len(rule.Middlewares))
	}

	// Default catch-all: the route-level middleware chain wrapping the backend.
	// Skip if a path rule already registered a catch-all (e.g. "/*" → "/{path...}").
	if !hasCatchAll {
		mux.HandleFunc("/", defaultHandler)
	}

	return &PathDispatcher{mux: mux}, mwNames, backends, nil
}

// buildPathBackendHandler creates the handler for a path rule that has its own
// backend.  It builds the reverse proxy, injects X-Forwarded-Prefix, and
// optionally strips the path prefix when explicitly requested.
func buildPathBackendHandler(rule config.PathRule) (http.HandlerFunc, error) {
	opts := proxyutil.ProxyOptions{
		DropQuery: rule.DropQuery != nil && *rule.DropQuery,
	}
	proxy, err := buildHttpRevProxy(rule.Backend, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build reverse proxy for path rule %q: %w", rule.Pattern, err)
	}
	handler := buildDefaultHttpHandler(proxy)

	// Inject X-Forwarded-Prefix so that backends aware of this header
	// (Spring Boot, ASP.NET, etc.) can generate correct absolute URLs
	// without manual base-URL configuration.
	if fwdPrefix := pathRulePrefix(rule.Pattern); fwdPrefix != "" {
		inner := handler
		handler = func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Forwarded-Prefix", fwdPrefix)
			inner(w, r)
		}
	}

	// Only strip the path prefix when explicitly requested.  By default the
	// full path (including the prefix) is forwarded so that response-generated
	// links (HTML, redirects, etc.) keep working without rewriting.  Most
	// self-hosted apps (Jellyfin, Sonarr, …) have a "base URL" setting the
	// user should configure to match the path prefix instead.
	if rule.StripPrefix != nil && *rule.StripPrefix {
		if prefix := pathRulePrefix(rule.Pattern); prefix != "" {
			zap.S().Infof("Stripping prefix %q for path rule %q", prefix, rule.Pattern)
			handler = http.StripPrefix(prefix, handler).ServeHTTP
		}
	}

	return handler, nil
}

// pathRulePrefix extracts the static prefix from a path-rule pattern so it
// can be stripped before forwarding to the backend.
//
// Examples:
//
//	/jellyfino    → /jellyfino
//	/jellyfino/   → /jellyfino
//	/jellyfino/*  → /jellyfino
//	/api/v1/      → /api/v1
//	/             → ""          (root – nothing to strip)
func pathRulePrefix(pattern string) string {
	p := strings.TrimSuffix(pattern, "*")
	p = strings.TrimRight(p, "/")
	if p == "" {
		return ""
	}
	// If the cleaned prefix still contains a wildcard (mid-path *), we
	// cannot use it with http.StripPrefix because the literal "*" would
	// never match a real request path segment.
	if strings.Contains(p, "*") {
		return ""
	}
	return p
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
