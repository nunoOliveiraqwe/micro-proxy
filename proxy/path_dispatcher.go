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

func buildPathDispatcher(ctx context.Context, defaultHandler http.HandlerFunc, pathRules []config.PathRule) (http.Handler, []string, []PathSnapshot, error) {
	mux := http.NewServeMux()

	backends := make([]string, 0)
	var pathSnapshots []PathSnapshot

	registeredPatternMap := make(map[string]struct{})

	for _, rule := range pathRules {
		pathBaseHandler := defaultHandler

		if rule.Backend != nil && rule.Backend.Address == "" {
			zap.S().Errorf("Path rule %q has a backend defined but no address specified. Skipping backend handler setup for this rule.", rule.Pattern)
			return nil, nil, nil, fmt.Errorf("path rule %q has a backend defined but no address specified", rule.Pattern)
		}

		pattern := normalizePattern(rule.Pattern)
		_, ok := registeredPatternMap[pattern]
		if ok {
			zap.S().Errorf("Duplicate path pattern detected: %q. Each path pattern must be unique. Please check your configuration.", pattern)
			//this seems not intentional, this is why it aborts
			return nil, nil, nil, fmt.Errorf("duplicate path pattern detected: %q", pattern)
		}

		if rule.Backend != nil && rule.Backend.Address != "" {
			zap.S().Infof("Building backend handler for path rule %q with backend %q", rule.Pattern, rule.Backend)
			handler, err := buildPathBackendHandler(rule)
			if err != nil {
				return nil, nil, nil, err
			}
			pathBaseHandler = handler
			backends = append(backends, rule.Backend.Address)
		}

		ctx2 := context.WithValue(ctx, ctxkeys.Path, rule.Pattern)

		handler, appliedMw, err := buildMiddlewareChain(ctx2, pathBaseHandler, rule.Middlewares, rule.DisableDefaults)
		if err != nil {
			return nil, nil, nil, err
		}

		handler = wrapTrustedProxies(ctx, handler, rule.TrustedProxies)

		// When the rule has its own backend, ensure the pattern covers
		// sub-paths too (e.g. "/jellyfino" → "/jellyfino/{path...}").
		if rule.Backend != nil && rule.Backend.Address != "" {
			pattern = ensureSubtree(pattern)
		}

		mux.HandleFunc(pattern, handler)
		registeredPatternMap[pattern] = struct{}{}

		backend := ""
		if rule.Backend != nil {
			backend = rule.Backend.Address
		}
		pathSnapshots = append(pathSnapshots, PathSnapshot{
			Pattern:     rule.Pattern,
			Backend:     backend,
			Middlewares: middlewareNames(appliedMw),
		})
		zap.S().Infof("Registered path rule %q with %d middlewares", pattern, len(appliedMw))
	}

	// Default catch-all: the route-level middleware chain wrapping the backend.
	// Skip if a path rule already registered a catch-all (e.g. "/*" → "/{path...}").
	_, hasCatchAll := registeredPatternMap["/"]
	_, hasCatchAllWild := registeredPatternMap["/{path...}"]
	if !hasCatchAll && !hasCatchAllWild {
		mux.HandleFunc("/", defaultHandler)
	}

	return &PathDispatcher{mux: mux}, backends, pathSnapshots, nil
}

// buildPathBackendHandler creates the handler for a path rule that has its own
// backend.  It builds the reverse proxy, injects X-Forwarded-Prefix, and
// optionally strips the path prefix when explicitly requested.
func buildPathBackendHandler(rule config.PathRule) (http.HandlerFunc, error) {
	opts := proxyutil.ProxyOptions{
		DropQuery:         rule.DropQuery != nil && *rule.DropQuery,
		ReplaceHostHeader: rule.Backend != nil && rule.Backend.ReplaceHostHeader,
	}
	proxy, err := buildHttpRevProxy(rule.Backend.Address, opts)
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
		if seg == "*" {
			if i == len(segments)-1 {
				segments[i] = "{path...}"
			} else {
				wildIdx++
				segments[i] = fmt.Sprintf("{_seg%d}", wildIdx)
			}
		} else if i == len(segments)-1 && strings.HasSuffix(seg, "*") {
			// Glued trailing star: "/api*" → "/api/{path...}"
			segments[i] = strings.TrimSuffix(seg, "*")
			segments = append(segments, "{path...}")
		}
	}
	return strings.Join(segments, "/")
}

func ensureSubtree(pattern string) string {
	if strings.HasSuffix(pattern, "/") || strings.Contains(pattern, "{path...}") {
		return pattern
	}
	return pattern + "/{path...}"
}
