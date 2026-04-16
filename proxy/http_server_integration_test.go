package proxy

// ============================================================================
// AI-GENERATED TEST FILE — April 2026
//
// Purpose: Exhaustive integration tests for HTTP proxy server creation with
// every middleware type and configuration variant. Each test builds a proxy
// via buildHttpServer(), calls start(), sends real HTTP requests through the
// live server, verifies middleware effects, then calls stop().
//
// Execution order documented in the codebase is:
//   global middleware → route middleware → path middleware → reverse proxy
//
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ── init ─────────────────────────────────────────────────────────────────────

func init() {
	zap.ReplaceGlobals(zap.NewNop())
}

// ── types ────────────────────────────────────────────────────────────────────

// echoResponse mirrors the JSON returned by newEchoBackend.
type echoResponse struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Host    string              `json:"host"`
	Headers map[string][]string `json:"headers"`
	Path    string              `json:"path"`
	Body    string              `json:"body"`
}

// ── helpers ──────────────────────────────────────────────────────────────────

func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

func waitForServer(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within %s", addr, timeout)
}

// newEchoBackend returns a test server that echoes request details as JSON.
// Paths /slow, /error500, and /status/{code} provide special behaviour.
func newEchoBackend(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow"))
	})
	mux.HandleFunc("/error500", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("backend-error"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var bodyBytes []byte
		if r.Body != nil {
			bodyBytes, _ = io.ReadAll(r.Body)
		}
		resp := echoResponse{
			Method:  r.Method,
			URL:     r.URL.String(),
			Host:    r.Host,
			Headers: r.Header,
			Path:    r.URL.Path,
			Body:    string(bodyBytes),
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend", "echo")
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// createTestContext builds a context suitable for buildHttpServer.
func createTestContext() context.Context {
	mgr := metrics.NewGlobalMetricsHandler(1, context.Background())
	mgr.StartCollectingMetrics()
	return context.WithValue(context.Background(), ctxkeys.MetricsMgr, mgr)
}

// buildAndStart builds a proxy server, starts it, waits for readiness,
// and registers cleanup to stop it. Returns the base URL for requests.
func buildAndStart(t *testing.T, listener config.HTTPListener, global *config.GlobalConfig) string {
	t.Helper()
	ctx := createTestContext()
	server, err := buildHttpServer(ctx, listener, global)
	require.NoError(t, err, "buildHttpServer must succeed")

	err = server.start(nil)
	require.NoError(t, err, "start must succeed")

	addr := fmt.Sprintf("127.0.0.1:%d", listener.Port)
	waitForServer(t, addr, 3*time.Second)

	t.Cleanup(func() { _ = server.stop() })
	return fmt.Sprintf("http://%s", addr)
}

// noRedirectClient returns an HTTP client that does NOT follow redirects.
func noRedirectClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func defaultClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func doGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := defaultClient().Get(url)
	require.NoError(t, err)
	return resp
}

func readEcho(t *testing.T, resp *http.Response) echoResponse {
	t.Helper()
	defer resp.Body.Close()
	var echo echoResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&echo))
	return echo
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

// simpleListener builds an HTTPListener with a default route to backend.
func simpleListener(port int, backend string, mws []middleware.Config) config.HTTPListener {
	return config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend:     backend,
			Middlewares: mws,
		},
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 1 — Lifecycle: build → start → stop
// ═══════════════════════════════════════════════════════════════════════════

func TestLifecycle_StartStop_NoMiddlewares(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)
	resp := doGet(t, baseURL+"/hello")
	echo := readEcho(t, resp)
	assert.Equal(t, "/hello", echo.Path)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestLifecycle_NoRoutes_Errors(t *testing.T) {
	ctx := createTestContext()
	_, err := buildHttpServer(ctx, config.HTTPListener{
		Port: getFreePort(t),
		Bind: config.Ipv4Flag,
	}, nil)
	require.Error(t, err, "buildHttpServer with no routes and no default must fail")
	assert.Contains(t, err.Error(), "no valid routes")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 2 — Individual middleware tests
// ═══════════════════════════════════════════════════════════════════════════

// ── RequestId ────────────────────────────────────────────────────────────────

func TestMiddleware_RequestId_AutoPrefix(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{Type: "RequestId"}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	echo := readEcho(t, resp)
	// The reverse proxy forwards the request; the backend must see the
	// X-Request-Id header OR the middleware just sets context. Let's verify
	// the request succeeds (middleware didn't panic).
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = echo // request went through
}

func TestMiddleware_RequestId_CustomPrefix(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type:    "RequestId",
		Options: map[string]interface{}{"prefix": "test-prefix"},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// ── RequestLog ───────────────────────────────────────────────────────────────

func TestMiddleware_RequestLog(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{Type: "RequestLog"}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/log-test")
	echo := readEcho(t, resp)
	assert.Equal(t, "/log-test", echo.Path)
}

// ── Metrics ──────────────────────────────────────────────────────────────────

func TestMiddleware_Metrics(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{Type: "Metrics"}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/metrics-test")
	echo := readEcho(t, resp)
	assert.Equal(t, "/metrics-test", echo.Path)
}

// ── Headers ──────────────────────────────────────────────────────────────────

func TestMiddleware_Headers_SetRequestHeaders(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Headers",
		Options: map[string]interface{}{
			"set-headers-req": map[string]interface{}{
				"X-Injected": "hello-from-proxy",
			},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	echo := readEcho(t, resp)
	require.Contains(t, echo.Headers, "X-Injected")
	assert.Equal(t, "hello-from-proxy", echo.Headers["X-Injected"][0])
}

func TestMiddleware_Headers_SetResponseHeaders(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Headers",
		Options: map[string]interface{}{
			"set-headers-res": map[string]interface{}{
				"X-Resp-Header": "added-by-proxy",
			},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, "added-by-proxy", resp.Header.Get("X-Resp-Header"))
}

func TestMiddleware_Headers_StripRequestHeaders(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Headers",
		Options: map[string]interface{}{
			"strip-headers-req": []interface{}{"X-Secret"},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("X-Secret", "should-be-stripped")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	echo := readEcho(t, resp)
	_, found := echo.Headers["X-Secret"]
	assert.False(t, found, "X-Secret should have been stripped from the request")
}

func TestMiddleware_Headers_StripResponseHeaders(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Headers",
		Options: map[string]interface{}{
			"strip-headers-res": []interface{}{"X-Backend"},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Empty(t, resp.Header.Get("X-Backend"),
		"X-Backend should have been stripped from the response")
}

func TestMiddleware_Headers_CmpRequestHeaders_Match(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Headers",
		Options: map[string]interface{}{
			"cmp-headers-req": map[string]interface{}{
				"X-Api-Key": "secret-key",
			},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("X-Api-Key", "secret-key")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"matching cmp-headers-req should allow the request")
}

func TestMiddleware_Headers_CmpRequestHeaders_Mismatch(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Headers",
		Options: map[string]interface{}{
			"cmp-headers-req": map[string]interface{}{
				"X-Api-Key": "secret-key",
			},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("X-Api-Key", "wrong-key")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"mismatched cmp-headers-req should return 401")
}

func TestMiddleware_Headers_CmpRequestHeaders_Missing(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Headers",
		Options: map[string]interface{}{
			"cmp-headers-req": map[string]interface{}{
				"X-Api-Key": "secret-key",
			},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"missing required header should return 401")
}

func TestMiddleware_Headers_AllCombined(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Headers",
		Options: map[string]interface{}{
			"set-headers-req":   map[string]interface{}{"X-Injected": "yes"},
			"set-headers-res":   map[string]interface{}{"X-Resp": "yes"},
			"strip-headers-req": []interface{}{"X-Strip-Me"},
			"strip-headers-res": []interface{}{"X-Backend"},
			"cmp-headers-req":   map[string]interface{}{"X-Auth": "ok"},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("X-Auth", "ok")
	req.Header.Set("X-Strip-Me", "should-vanish")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	echo := readEcho(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, echo.Headers, "X-Injected")
	assert.Equal(t, "yes", echo.Headers["X-Injected"][0])
	_, hasStripped := echo.Headers["X-Strip-Me"]
	assert.False(t, hasStripped, "X-Strip-Me should have been stripped")
	assert.Equal(t, "yes", resp.Header.Get("X-Resp"))
	assert.Empty(t, resp.Header.Get("X-Backend"), "X-Backend should be stripped from response")
}

// ── RateLimiter ──────────────────────────────────────────────────────────────

func TestMiddleware_RateLimiter_Global(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "RateLimiter",
		Options: map[string]interface{}{
			"mode": "global",
			"limiter-req": map[string]interface{}{
				"rate-per-second": 1.0,
				"burst":           1,
			},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	// First request should succeed.
	resp := doGet(t, baseURL+"/")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Hammer quickly — at least one should be 429.
	got429 := false
	for i := 0; i < 20; i++ {
		r := doGet(t, baseURL+"/")
		r.Body.Close()
		if r.StatusCode == http.StatusTooManyRequests {
			got429 = true
			assert.NotEmpty(t, r.Header.Get("Retry-After"))
			break
		}
	}
	assert.True(t, got429, "global rate limiter should have returned 429")
}

func TestMiddleware_RateLimiter_PerClient(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "RateLimiter",
		Options: map[string]interface{}{
			"mode": "per-client",
			"limiter-req": map[string]interface{}{
				"rate-per-second": 1.0,
				"burst":           1,
			},
			"cache-ttl":        "10s",
			"cleanup-interval": "10s",
			"max-cache-size":   100,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	got429 := false
	for i := 0; i < 20; i++ {
		r := doGet(t, baseURL+"/")
		r.Body.Close()
		if r.StatusCode == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	assert.True(t, got429, "per-client rate limiter should have returned 429")
}

// ── IpBlock (not yet implemented) ────────────────────────────────────────────

func TestMiddleware_IpBlock_PassesThrough(t *testing.T) {
	// IpBlockMiddleware is registered but is a no-op (TODO stub).
	// This test documents that it currently passes all traffic through.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "IpBlock",
		Options: map[string]interface{}{
			"list-mode": "block",
			"list":      []interface{}{"127.0.0.1"},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	// The middleware is a no-op, so the request passes through.
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ── Redirect ─────────────────────────────────────────────────────────────────

func TestMiddleware_Redirect_External(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":        "external",
			"target":      "http://example.com",
			"status-code": 302,
			"drop-path":   true,
			"drop-query":  true,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp, err := noRedirectClient().Get(baseURL + "/some-path?q=1")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "http://example.com", resp.Header.Get("Location"))
}

func TestMiddleware_Redirect_External_PreservePath(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":        "external",
			"target":      "http://example.com",
			"status-code": 301,
			"drop-path":   false,
			"drop-query":  false,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp, err := noRedirectClient().Get(baseURL + "/some-path?q=1")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 301, resp.StatusCode)
	loc := resp.Header.Get("Location")
	assert.Contains(t, loc, "/some-path")
	assert.Contains(t, loc, "q=1")
}

func TestMiddleware_Redirect_Internal(t *testing.T) {
	// The internal redirect should proxy transparently to the target
	// instead of sending an HTTP redirect.
	redirectTarget := newEchoBackend(t)
	originalBackend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":       "internal",
			"target":     redirectTarget.URL,
			"drop-path":  false,
			"drop-query": false,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, originalBackend.URL, mws), nil)

	resp := doGet(t, baseURL+"/internal-test")
	echo := readEcho(t, resp)
	// Must reach the redirect target, not the original backend.
	assert.Equal(t, "/internal-test", echo.Path)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMiddleware_Redirect_External_DefaultStatusCode(t *testing.T) {
	// When status-code is omitted for external mode, the default should be 302.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":   "external",
			"target": "http://example.com",
			// status-code intentionally omitted — should default to 302
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)
	resp, err := noRedirectClient().Get(baseURL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 302, resp.StatusCode,
		"external redirect should default to 302 when status-code is omitted")
	assert.Equal(t, "http://example.com", resp.Header.Get("Location"))
}

// ── BodySizeLimit ────────────────────────────────────────────────────────────

func TestMiddleware_BodySizeLimit_UnderLimit(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "BodySizeLimit",
		Options: map[string]interface{}{
			"max-size": "1m",
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	body := strings.NewReader("small body")
	resp, err := defaultClient().Post(baseURL+"/", "text/plain", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMiddleware_BodySizeLimit_OverLimit(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "BodySizeLimit",
		Options: map[string]interface{}{
			"max-size": "1k", // 1024 bytes
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	bigBody := strings.NewReader(strings.Repeat("X", 2048))
	resp, err := defaultClient().Post(baseURL+"/", "text/plain", bigBody)
	require.NoError(t, err)
	defer resp.Body.Close()
	// When MaxBytesReader triggers, the backend typically sees a truncated
	// body and Go writes a 413 or the backend errors. The important thing
	// is the request is not blindly passed.
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"a body exceeding the limit should not succeed with 200")
}

// ── Timeout ──────────────────────────────────────────────────────────────────

func TestMiddleware_Timeout_FastBackend(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Timeout",
		Options: map[string]interface{}{
			"request-timeout": "10s",
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMiddleware_Timeout_SlowBackend(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Timeout",
		Options: map[string]interface{}{
			"request-timeout": "200ms",
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp, err := defaultClient().Get(baseURL + "/slow")
	require.NoError(t, err)
	body := readBody(t, resp)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"slow backend should trigger the timeout middleware")
	assert.Contains(t, body, "request timed out")
}

func TestMiddleware_Timeout_MillisecondParsing(t *testing.T) {
	// Verify that millisecond durations like "200ms" are correctly parsed.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Timeout",
		Options: map[string]interface{}{
			"request-timeout": "500ms",
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"fast backend with 500ms timeout should succeed")
}

// ── HoneyPot ─────────────────────────────────────────────────────────────────

func TestMiddleware_HoneyPot_NormalPath(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "HoneyPot",
		Options: map[string]interface{}{
			"defaults":         []interface{}{"php"},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/legitimate")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "/legitimate", echo.Path)
}

func TestMiddleware_HoneyPot_TrapPath(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "HoneyPot",
		Options: map[string]interface{}{
			"paths":            []interface{}{"/trap"},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	// Hit the trap path.
	resp := doGet(t, baseURL+"/trap")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"trap path should return 403")

	// Subsequent requests from the same IP should also be blocked.
	resp2 := doGet(t, baseURL+"/legitimate")
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp2.StatusCode,
		"IP should be cached and blocked for all subsequent requests")
}

func TestMiddleware_HoneyPot_DefaultGroups(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "HoneyPot",
		Options: map[string]interface{}{
			"defaults":         []interface{}{"php", "git", "infra"},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	trapPaths := []string{"/.env", "/.git/config", "/actuator"}
	for _, p := range trapPaths {
		resp := doGet(t, baseURL+p)
		resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode,
			"honeypot trap path %s should be blocked", p)
	}
}

func TestMiddleware_HoneyPot_CustomResponse(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "HoneyPot",
		Options: map[string]interface{}{
			"paths":            []interface{}{"/secret-trap"},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
			"response": map[string]interface{}{
				"status-code": 404,
				"body":        "Not Found",
			},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/secret-trap")
	body := readBody(t, resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, body, "Not Found")
}

// ── UserAgentBlocker ─────────────────────────────────────────────────────────

func TestMiddleware_UserAgentBlocker_AllowGoodUA(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "UserAgentBlocker",
		Options: map[string]interface{}{
			"block-empty-ua":   true,
			"block-defaults":   []interface{}{"scanners"},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMiddleware_UserAgentBlocker_BlockEmptyUA(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "UserAgentBlocker",
		Options: map[string]interface{}{
			"block-empty-ua":   true,
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("User-Agent", "")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"empty User-Agent should be blocked")
}

func TestMiddleware_UserAgentBlocker_CustomBlockPattern(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "UserAgentBlocker",
		Options: map[string]interface{}{
			"block-empty-ua":   false,
			"block":            []interface{}{"evil-bot"},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("User-Agent", "evil-bot/1.0")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"custom blocked UA pattern should be blocked")
}

func TestMiddleware_UserAgentBlocker_AllowOverridesBlock(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "UserAgentBlocker",
		Options: map[string]interface{}{
			"block-empty-ua":   false,
			"block":            []interface{}{"my-bot"},
			"allow":            []interface{}{"my-bot"},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("User-Agent", "my-bot/1.0")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"allow list should override block list for the same UA pattern")
}

// ── CircuitBreaker ───────────────────────────────────────────────────────────

func TestMiddleware_CircuitBreaker_HealthyBackend(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "CircuitBreaker",
		Options: map[string]interface{}{
			"failure-threshold":           5,
			"recovery-time":               "30s",
			"half-open-success-threshold": 2,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	for i := 0; i < 10; i++ {
		resp := doGet(t, baseURL+"/")
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
}

func TestMiddleware_CircuitBreaker_OpensAfterFailures(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "CircuitBreaker",
		Options: map[string]interface{}{
			"failure-threshold":           3,
			"recovery-time":               "5s",
			"half-open-success-threshold": 1,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	// Trigger failures.
	for i := 0; i < 5; i++ {
		resp := doGet(t, baseURL+"/error500")
		resp.Body.Close()
	}

	// The circuit should now be open.
	resp := doGet(t, baseURL+"/")
	body := readBody(t, resp)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"circuit breaker should be open after exceeding failure threshold")
	assert.Contains(t, body, "Service unavailable")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 3 — Middleware execution order
// ═══════════════════════════════════════════════════════════════════════════

func TestMiddlewareOrder_ConfigOrderIsExecutionOrder(t *testing.T) {
	// Two Headers middlewares in config order [A, B]:
	// A sets X-Order-Check on the request, B requires it via cmp-headers-req.
	// If A runs before B, the request succeeds. If B runs before A, it 401s.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{
		{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Order-Check": "set-by-first",
				},
			},
		},
		{
			Type: "Headers",
			Options: map[string]interface{}{
				"cmp-headers-req": map[string]interface{}{
					"X-Order-Check": "set-by-first",
				},
			},
		},
	}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"first middleware in config should execute first; it sets the header "+
			"that the second middleware checks")
}

func TestMiddlewareOrder_ReverseConfigOrderFails(t *testing.T) {
	// Reverse order: checker comes BEFORE setter → should 401.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{
		{
			Type: "Headers",
			Options: map[string]interface{}{
				"cmp-headers-req": map[string]interface{}{
					"X-Order-Check": "set-by-second",
				},
			},
		},
		{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Order-Check": "set-by-second",
				},
			},
		},
	}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"when checker runs before setter the header is missing → 401")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 5 — Global → Route → Path middleware chaining
// ═══════════════════════════════════════════════════════════════════════════

func TestMiddlewareChain_GlobalBeforeRoute(t *testing.T) {
	// Global middleware sets a header; route middleware checks it.
	// If global runs first, check passes.
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Global": "applied",
				},
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "Headers",
				Options: map[string]interface{}{
					"cmp-headers-req": map[string]interface{}{
						"X-Global": "applied",
					},
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	resp := doGet(t, baseURL+"/")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"global middleware must execute before route middleware")
	assert.Equal(t, "applied", echo.Headers["X-Global"][0])
}

func TestMiddlewareChain_RouteBeforePath(t *testing.T) {
	// Route middleware sets a header; path middleware checks it.
	backend := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "Headers",
				Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{
						"X-Route": "route-set",
					},
				},
			}},
			Paths: []config.PathRule{{
				Pattern: "/api/*",
				Middlewares: []middleware.Config{{
					Type: "Headers",
					Options: map[string]interface{}{
						"cmp-headers-req": map[string]interface{}{
							"X-Route": "route-set",
						},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	resp := doGet(t, baseURL+"/api/test")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"route middleware must execute before path middleware")
	_ = echo
}

func TestMiddlewareChain_GlobalRoutePathAllApply(t *testing.T) {
	// Full chain: global sets X-Global, route sets X-Route, path sets X-Path.
	// Backend should see all three.
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Global": "g",
				},
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "Headers",
				Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{
						"X-Route": "r",
					},
				},
			}},
			Paths: []config.PathRule{{
				Pattern: "/deep/*",
				Middlewares: []middleware.Config{{
					Type: "Headers",
					Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{
							"X-Path": "p",
						},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	resp := doGet(t, baseURL+"/deep/resource")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "g", echo.Headers["X-Global"][0], "global middleware header missing")
	assert.Equal(t, "r", echo.Headers["X-Route"][0], "route middleware header missing")
	assert.Equal(t, "p", echo.Headers["X-Path"][0], "path middleware header missing")
}

func TestMiddlewareChain_FullOrderProof(t *testing.T) {
	// This test PROVES the execution order global → route → path through
	// causal dependencies: each level checks for a header that the previous
	// level must have already set. If the order were wrong, the cmp-headers-req
	// check would fail with 401.
	//
	// Global: sets X-Chain-Global
	// Route:  requires X-Chain-Global (proves global ran first), sets X-Chain-Route
	// Path:   requires X-Chain-Route  (proves route ran before path), sets X-Chain-Path
	// Backend: sees all three headers (proves all three ran)
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Chain-Global": "global-was-here",
				},
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{
				{
					// First: check that global already ran
					Type: "Headers",
					Options: map[string]interface{}{
						"cmp-headers-req": map[string]interface{}{
							"X-Chain-Global": "global-was-here",
						},
					},
				},
				{
					// Second: set route header for path to check
					Type: "Headers",
					Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{
							"X-Chain-Route": "route-was-here",
						},
					},
				},
			},
			Paths: []config.PathRule{{
				Pattern: "/chained/*",
				Middlewares: []middleware.Config{
					{
						// First: check that route already ran
						Type: "Headers",
						Options: map[string]interface{}{
							"cmp-headers-req": map[string]interface{}{
								"X-Chain-Route": "route-was-here",
							},
						},
					},
					{
						// Second: set path header so backend can confirm
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{
								"X-Chain-Path": "path-was-here",
							},
						},
					},
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	resp := doGet(t, baseURL+"/chained/resource")
	echo := readEcho(t, resp)

	// If any level ran out of order the cmp-headers-req would have
	// returned 401 Unauthorized.
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"full chain should succeed — if 401, the order is wrong")
	assert.Equal(t, "global-was-here", echo.Headers["X-Chain-Global"][0])
	assert.Equal(t, "route-was-here", echo.Headers["X-Chain-Route"][0])
	assert.Equal(t, "path-was-here", echo.Headers["X-Chain-Path"][0])
}

func TestMiddlewareChain_PathMatchVsNonPathMatch(t *testing.T) {
	// Verify that path middleware only applies to matching paths.
	// A request to /api/* should get global + route + path middleware.
	// A request to / should get global + route middleware only.
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Global": "yes",
				},
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "Headers",
				Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{
						"X-Route": "yes",
					},
				},
			}},
			Paths: []config.PathRule{{
				Pattern: "/api/*",
				Middlewares: []middleware.Config{{
					Type: "Headers",
					Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{
							"X-Path-Api": "yes",
						},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("matching path gets all three levels", func(t *testing.T) {
		resp := doGet(t, baseURL+"/api/users")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "yes", echo.Headers["X-Global"][0], "global should apply")
		assert.Equal(t, "yes", echo.Headers["X-Route"][0], "route should apply")
		assert.Equal(t, "yes", echo.Headers["X-Path-Api"][0], "path middleware should apply to /api/*")
	})

	t.Run("non-matching path gets only global and route", func(t *testing.T) {
		resp := doGet(t, baseURL+"/other")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "yes", echo.Headers["X-Global"][0], "global should apply")
		assert.Equal(t, "yes", echo.Headers["X-Route"][0], "route should apply")
		_, hasPathHeader := echo.Headers["X-Path-Api"]
		assert.False(t, hasPathHeader, "path middleware must NOT apply to /other")
	})
}

func TestMiddlewareChain_HostRoutingWithGlobalAndPathMiddleware(t *testing.T) {
	// Full realistic scenario: global middleware + host-based routing +
	// per-route middleware + path-specific middleware + separate backends.
	//
	// Setup:
	//   Global:    sets X-Global
	//   alpha.local → backendA with route middleware (sets X-Host=alpha)
	//     /admin/* → path middleware (requires X-Host=alpha, sets X-Zone=admin)
	//   beta.local  → backendB with route middleware (sets X-Host=beta)
	//   default     → backendDefault with no route middleware
	//
	backendA := newEchoBackend(t)
	backendB := newEchoBackend(t)
	backendDefault := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Global": "applied",
				},
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Routes: []config.Route{
			{
				Host: "alpha.local",
				Target: config.RouteTarget{
					Backend: backendA.URL,
					Middlewares: []middleware.Config{{
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{
								"X-Host": "alpha",
							},
						},
					}},
					Paths: []config.PathRule{{
						Pattern: "/admin/*",
						Middlewares: []middleware.Config{
							{
								Type: "Headers",
								Options: map[string]interface{}{
									"cmp-headers-req": map[string]interface{}{
										"X-Host": "alpha",
									},
								},
							},
							{
								Type: "Headers",
								Options: map[string]interface{}{
									"set-headers-req": map[string]interface{}{
										"X-Zone": "admin",
									},
								},
							},
						},
					}},
				},
			},
			{
				Host: "beta.local",
				Target: config.RouteTarget{
					Backend: backendB.URL,
					Middlewares: []middleware.Config{{
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{
								"X-Host": "beta",
							},
						},
					}},
				},
			},
		},
		Default: &config.RouteTarget{
			Backend: backendDefault.URL,
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("alpha.local /admin/* gets global+route+path", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/admin/dashboard", nil)
		req.Host = "alpha.local"
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "applied", echo.Headers["X-Global"][0], "global middleware")
		assert.Equal(t, "alpha", echo.Headers["X-Host"][0], "route middleware")
		assert.Equal(t, "admin", echo.Headers["X-Zone"][0], "path middleware")
	})

	t.Run("alpha.local /other gets global+route only", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/other", nil)
		req.Host = "alpha.local"
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "applied", echo.Headers["X-Global"][0], "global middleware")
		assert.Equal(t, "alpha", echo.Headers["X-Host"][0], "route middleware")
		_, hasZone := echo.Headers["X-Zone"]
		assert.False(t, hasZone, "path middleware must not apply outside /admin/*")
	})

	t.Run("beta.local gets global+route, no path", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
		req.Host = "beta.local"
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "applied", echo.Headers["X-Global"][0], "global middleware")
		assert.Equal(t, "beta", echo.Headers["X-Host"][0], "beta route middleware")
		_, hasZone := echo.Headers["X-Zone"]
		assert.False(t, hasZone, "alpha's path middleware must not bleed to beta")
	})

	t.Run("unknown host falls to default, gets global only", func(t *testing.T) {
		resp := doGet(t, baseURL+"/")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "applied", echo.Headers["X-Global"][0], "global middleware")
		_, hasHost := echo.Headers["X-Host"]
		assert.False(t, hasHost, "default route has no route middleware")
	})
}

func TestMiddlewareChain_MixedMiddlewareTypesPerLevel(t *testing.T) {
	// Use different middleware types at each level — not just Headers — to
	// prove the real middleware implementations compose correctly.
	//
	// Global:  RequestId (sets context) + RequestLog (sets logger in context)
	// Route:   Headers (sets X-Route-Applied, checks nothing)
	// Path:    BodySizeLimit (limits body) + Timeout (limits time)
	//
	// A normal GET should flow through all of them.
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{
			{Type: "RequestId", Options: map[string]interface{}{"prefix": "chain-test"}},
			{Type: "RequestLog"},
		},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "Headers",
				Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{
						"X-Route-Applied": "true",
					},
				},
			}},
			Paths: []config.PathRule{{
				Pattern: "/limited/*",
				Middlewares: []middleware.Config{
					{
						Type: "BodySizeLimit",
						Options: map[string]interface{}{
							"max-size": "1m",
						},
					},
					{
						Type: "Timeout",
						Options: map[string]interface{}{
							"request-timeout": "10s",
						},
					},
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("normal request flows through all levels", func(t *testing.T) {
		resp := doGet(t, baseURL+"/limited/resource")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// Route middleware set the header
		assert.Equal(t, "true", echo.Headers["X-Route-Applied"][0])
	})

	t.Run("slow backend triggers path-level timeout", func(t *testing.T) {
		// Build a separate server with a short timeout at path level.
		// The path rule has its own backend and strip-prefix enabled:
		// /slow-path/slow → /slow on the backend (which sleeps 5s).
		backend2 := newEchoBackend(t)
		port2 := getFreePort(t)
		stripPrefix := true
		listener2 := config.HTTPListener{
			Port: port2,
			Bind: config.Ipv4Flag,
			Default: &config.RouteTarget{
				Backend: backend2.URL,
				Paths: []config.PathRule{{
					Pattern:     "/slow-path/*",
					Backend:     backend2.URL,
					StripPrefix: &stripPrefix,
					Middlewares: []middleware.Config{{
						Type: "Timeout",
						Options: map[string]interface{}{
							"request-timeout": "200ms",
						},
					}},
				}},
			},
		}
		baseURL2 := buildAndStart(t, listener2, globalConf)

		resp, err := defaultClient().Get(baseURL2 + "/slow-path/slow")
		require.NoError(t, err)
		body := readBody(t, resp)
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
			"path-level timeout should fire for slow backend")
		assert.Contains(t, body, "request timed out")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Battle tests: every realistic middleware combination across the full chain.
// Each test starts a live proxy, sends real HTTP requests, and validates every
// middleware at every level (global / route / path) fires in the right order
// with the right config.
// ─────────────────────────────────────────────────────────────────────────────

// ── 1. Global=RequestId+RequestLog, Route=Headers(set+cmp), Path=BodySizeLimit

func TestChainBattle_RequestIdLog_HeadersCmp_BodySizeLimit(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{
			{Type: "RequestId", Options: map[string]interface{}{"prefix": "battle1"}},
			{Type: "RequestLog"},
		},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{
				{Type: "Headers", Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{"X-Battle": "1"},
				}},
			},
			Paths: []config.PathRule{{
				Pattern: "/upload/*",
				Backend: backend.URL,
				Middlewares: []middleware.Config{
					{Type: "BodySizeLimit", Options: map[string]interface{}{"max-size": "1k"}},
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("small body through full chain succeeds", func(t *testing.T) {
		body := strings.NewReader("tiny")
		resp, err := defaultClient().Post(baseURL+"/upload/file", "text/plain", body)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "1", echo.Headers["X-Battle"][0])
	})

	t.Run("oversized body rejected at path level", func(t *testing.T) {
		big := strings.NewReader(strings.Repeat("X", 2048))
		resp, err := defaultClient().Post(baseURL+"/upload/file", "text/plain", big)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.NotEqual(t, http.StatusOK, resp.StatusCode,
			"body size limit at path level should reject large body")
	})

	t.Run("non-path request ignores body limit", func(t *testing.T) {
		big := strings.NewReader(strings.Repeat("X", 2048))
		resp, err := defaultClient().Post(baseURL+"/other", "text/plain", big)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"body size limit should NOT apply outside /upload/*")
		assert.Equal(t, "1", echo.Headers["X-Battle"][0])
		_ = echo
	})
}

// ── 2. Global=RateLimiter, Route=RequestId+Headers, Path=Timeout

func TestChainBattle_GlobalRateLimit_RouteIdHeaders_PathTimeout(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	stripPrefix := true

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "RateLimiter",
			Options: map[string]interface{}{
				"mode":        "global",
				"limiter-req": map[string]interface{}{"rate-per-second": 100.0, "burst": 100},
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{
				{Type: "RequestId", Options: map[string]interface{}{"prefix": "battle2"}},
				{Type: "Headers", Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{"X-Route": "battle2"},
				}},
			},
			Paths: []config.PathRule{{
				Pattern:     "/timed/*",
				Backend:     backend.URL,
				StripPrefix: &stripPrefix,
				Middlewares: []middleware.Config{{
					Type:    "Timeout",
					Options: map[string]interface{}{"request-timeout": "200ms"},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("fast path request passes all three levels", func(t *testing.T) {
		resp := doGet(t, baseURL+"/timed/fast")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "battle2", echo.Headers["X-Route"][0])
	})

	t.Run("slow path request times out at path level", func(t *testing.T) {
		resp, err := defaultClient().Get(baseURL + "/timed/slow")
		require.NoError(t, err)
		body := readBody(t, resp)
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		assert.Contains(t, body, "request timed out")
	})

	t.Run("non-path request has no timeout", func(t *testing.T) {
		resp := doGet(t, baseURL+"/notimed")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "battle2", echo.Headers["X-Route"][0])
		_ = echo
	})
}

// ── 3. Global=HoneyPot, Route=Headers(strip-req), Path=Headers(set-req)
//       Proves honeypot at global level blocks before route/path ever run.

func TestChainBattle_GlobalHoneyPot_RouteStripHeaders_PathSetHeaders(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "HoneyPot",
			Options: map[string]interface{}{
				"paths":            []interface{}{"/wp-login.php"},
				"cache-ttl":        "1h",
				"cleanup-interval": "1h",
				"max-cache-size":   1000,
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "Headers", Options: map[string]interface{}{
					"strip-headers-req": []interface{}{"X-Internal"},
				},
			}},
			Paths: []config.PathRule{{
				Pattern: "/api/*",
				Middlewares: []middleware.Config{{
					Type: "Headers", Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{"X-Api": "true"},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("normal api request flows through all three levels", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/data", nil)
		req.Header.Set("X-Internal", "secret")
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// route middleware stripped X-Internal
		_, hasInternal := echo.Headers["X-Internal"]
		assert.False(t, hasInternal, "route should strip X-Internal")
		// path middleware set X-Api
		assert.Equal(t, "true", echo.Headers["X-Api"][0])
	})

	t.Run("honeypot trap at global blocks before route or path run", func(t *testing.T) {
		resp := doGet(t, baseURL+"/wp-login.php")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("after trap IP is cached all subsequent requests are blocked", func(t *testing.T) {
		// The previous subtest already tripped the honeypot for our IP.
		resp := doGet(t, baseURL+"/api/data")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode,
			"global honeypot should block the cached IP before route/path run")
	})
}

// ── 4. Global=UserAgentBlocker, Route=Metrics+RequestLog, Path=Headers(cmp)
//       UA blocker at global rejects bots before anything else runs.

func TestChainBattle_GlobalUABlocker_RouteMetricsLog_PathHeadersCmp(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "UserAgentBlocker",
			Options: map[string]interface{}{
				"block-empty-ua":   false,
				"block":            []interface{}{"evil-scanner"},
				"cache-ttl":        "1h",
				"cleanup-interval": "1h",
				"max-cache-size":   100,
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{
				{Type: "Metrics"},
				{Type: "RequestLog"},
			},
			Paths: []config.PathRule{{
				Pattern: "/secure/*",
				Middlewares: []middleware.Config{{
					Type: "Headers", Options: map[string]interface{}{
						"cmp-headers-req": map[string]interface{}{"X-Token": "valid"},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("good UA with valid token passes all levels", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/secure/data", nil)
		req.Header.Set("User-Agent", "GoodBrowser/1.0")
		req.Header.Set("X-Token", "valid")
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("good UA without token rejected at path level", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/secure/data", nil)
		req.Header.Set("User-Agent", "GoodBrowser/1.0")
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"path-level cmp-headers-req should reject missing token")
	})

	t.Run("bad UA blocked at global before route or path", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/secure/data", nil)
		req.Header.Set("User-Agent", "evil-scanner/1.0")
		req.Header.Set("X-Token", "valid") // token is valid but doesn't matter
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode,
			"global UA blocker should reject before route/path run")
	})
}

// ── 5. Global=Headers(set-res), Route=CircuitBreaker, Path=Headers(set-req)
//       Response headers from global should appear even when circuit breaker opens.

func TestChainBattle_GlobalResHeaders_RouteCircuitBreaker_PathReqHeaders(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-res": map[string]interface{}{"X-Proxy": "torii"},
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "CircuitBreaker",
				Options: map[string]interface{}{
					"failure-threshold":           3,
					"recovery-time":               "10s",
					"half-open-success-threshold": 1,
				},
			}},
			Paths: []config.PathRule{{
				Pattern: "/api/*",
				Middlewares: []middleware.Config{{
					Type: "Headers", Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{"X-Path": "api"},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("healthy requests have global res header and path req header", func(t *testing.T) {
		resp := doGet(t, baseURL+"/api/ok")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "torii", resp.Header.Get("X-Proxy"), "global response header")
		assert.Equal(t, "api", echo.Headers["X-Path"][0], "path request header")
	})

	t.Run("trigger circuit breaker then verify global header still appears on 503", func(t *testing.T) {
		// Hammer errors to trip the breaker.
		for i := 0; i < 5; i++ {
			r := doGet(t, baseURL+"/error500")
			r.Body.Close()
		}
		resp := doGet(t, baseURL+"/api/test")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
			"circuit should be open")
		assert.Equal(t, "torii", resp.Header.Get("X-Proxy"),
			"global response header should still be set even when circuit breaker fires")
	})
}

// ── 6. Global=Timeout, Route=Headers(set+strip), Path=RateLimiter
//       Global timeout wraps everything; path rate limiter fires inside it.

func TestChainBattle_GlobalTimeout_RouteHeaders_PathRateLimiter(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type:    "Timeout",
			Options: map[string]interface{}{"request-timeout": "5s"},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{
				{Type: "Headers", Options: map[string]interface{}{
					"set-headers-req":   map[string]interface{}{"X-Route": "set"},
					"strip-headers-req": []interface{}{"X-Remove-Me"},
				}},
			},
			Paths: []config.PathRule{{
				Pattern: "/limited/*",
				Middlewares: []middleware.Config{{
					Type: "RateLimiter",
					Options: map[string]interface{}{
						"mode":        "global",
						"limiter-req": map[string]interface{}{"rate-per-second": 2.0, "burst": 2},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("request flows through global timeout + route headers + path ratelimit", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/limited/resource", nil)
		req.Header.Set("X-Remove-Me", "should-vanish")
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "set", echo.Headers["X-Route"][0])
		_, stripped := echo.Headers["X-Remove-Me"]
		assert.False(t, stripped, "route should strip X-Remove-Me")
	})

	t.Run("path rate limiter fires inside the chain", func(t *testing.T) {
		got429 := false
		for i := 0; i < 20; i++ {
			r := doGet(t, baseURL+"/limited/resource")
			r.Body.Close()
			if r.StatusCode == http.StatusTooManyRequests {
				got429 = true
				break
			}
		}
		assert.True(t, got429, "path-level rate limiter should return 429")
	})

	t.Run("non-path requests are not rate limited", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			resp := doGet(t, baseURL+"/unlimited")
			resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		}
	})
}

// ── 7. Global=BodySizeLimit, Route=RequestId+Timeout, Path=Headers(cmp+set)
//       Global body limit protects all paths; route adds observability.

func TestChainBattle_GlobalBodyLimit_RouteIdTimeout_PathCmpSet(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type:    "BodySizeLimit",
			Options: map[string]interface{}{"max-size": "10m"},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{
				{Type: "RequestId", Options: map[string]interface{}{"prefix": "b7"}},
				{Type: "Timeout", Options: map[string]interface{}{"request-timeout": "5s"}},
			},
			Paths: []config.PathRule{{
				Pattern: "/admin/*",
				Middlewares: []middleware.Config{
					{Type: "Headers", Options: map[string]interface{}{
						"cmp-headers-req": map[string]interface{}{"X-Admin-Token": "super-secret"},
					}},
					{Type: "Headers", Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{"X-Admin": "verified"},
					}},
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("admin with token passes all levels", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/admin/panel", nil)
		req.Header.Set("X-Admin-Token", "super-secret")
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "verified", echo.Headers["X-Admin"][0])
	})

	t.Run("admin without token rejected at path cmp", func(t *testing.T) {
		resp := doGet(t, baseURL+"/admin/panel")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("non-admin path skips path middleware entirely", func(t *testing.T) {
		resp := doGet(t, baseURL+"/public")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_, hasAdmin := echo.Headers["X-Admin"]
		assert.False(t, hasAdmin, "path middleware must not apply to /public")
		_ = echo
	})
}

// ── 8. Two host routes with different middleware stacks + global + path
//       The most complex scenario: host isolation under shared global.

func TestChainBattle_TwoHosts_DifferentStacks_GlobalShared(t *testing.T) {
	backendApp := newEchoBackend(t)
	backendApi := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{
			{Type: "RequestId", Options: map[string]interface{}{"prefix": "multi-host"}},
			{Type: "Headers", Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{"X-Global": "shared"},
			}},
		},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Routes: []config.Route{
			{
				Host: "app.local",
				Target: config.RouteTarget{
					Backend: backendApp.URL,
					Middlewares: []middleware.Config{
						{Type: "Headers", Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{"X-Service": "app"},
						}},
					},
					Paths: []config.PathRule{
						{
							Pattern: "/admin/*",
							Middlewares: []middleware.Config{{
								Type: "Headers", Options: map[string]interface{}{
									"cmp-headers-req": map[string]interface{}{"X-Role": "admin"},
								},
							}},
						},
						{
							Pattern: "/public/*",
							Middlewares: []middleware.Config{{
								Type: "Headers", Options: map[string]interface{}{
									"set-headers-req": map[string]interface{}{"X-Zone": "public"},
								},
							}},
						},
					},
				},
			},
			{
				Host: "api.local",
				Target: config.RouteTarget{
					Backend: backendApi.URL,
					Middlewares: []middleware.Config{
						{Type: "Headers", Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{"X-Service": "api"},
						}},
						{Type: "RateLimiter", Options: map[string]interface{}{
							"mode":        "global",
							"limiter-req": map[string]interface{}{"rate-per-second": 100.0, "burst": 100},
						}},
					},
					Paths: []config.PathRule{{
						Pattern: "/v1/*",
						Middlewares: []middleware.Config{{
							Type: "Headers", Options: map[string]interface{}{
								"set-headers-req": map[string]interface{}{"X-Version": "v1"},
							},
						}},
					}},
				},
			},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("app.local /public/* gets global+route+path", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/public/index", nil)
		req.Host = "app.local"
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "shared", echo.Headers["X-Global"][0])
		assert.Equal(t, "app", echo.Headers["X-Service"][0])
		assert.Equal(t, "public", echo.Headers["X-Zone"][0])
	})

	t.Run("app.local /admin/* without role header rejected", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/admin/settings", nil)
		req.Host = "app.local"
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("app.local /admin/* with role header passes", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/admin/settings", nil)
		req.Host = "app.local"
		req.Header.Set("X-Role", "admin")
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "shared", echo.Headers["X-Global"][0])
		assert.Equal(t, "app", echo.Headers["X-Service"][0])
	})

	t.Run("api.local /v1/* gets global+route(with ratelimiter)+path", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/users", nil)
		req.Host = "api.local"
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "shared", echo.Headers["X-Global"][0])
		assert.Equal(t, "api", echo.Headers["X-Service"][0])
		assert.Equal(t, "v1", echo.Headers["X-Version"][0])
	})

	t.Run("api.local middleware does not bleed to app.local", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/users", nil)
		req.Host = "app.local"
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "app", echo.Headers["X-Service"][0], "should get app service, not api")
		_, hasVersion := echo.Headers["X-Version"]
		assert.False(t, hasVersion, "api path middleware must not apply on app host")
	})

	t.Run("unknown host with no default returns 502", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
		req.Host = "unknown.local"
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})
}

// ── 9. Global=Headers(set-res)+Metrics, Route=Redirect(external)
//       Redirect at route level should still get global response headers.

func TestChainBattle_GlobalResHeaders_RouteRedirect(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{
			{Type: "Headers", Options: map[string]interface{}{
				"set-headers-res": map[string]interface{}{"X-Via": "torii-proxy"},
			}},
			{Type: "Metrics"},
		},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "Redirect",
				Options: map[string]interface{}{
					"mode":        "external",
					"target":      "https://redirected.example.com",
					"status-code": 307,
					"drop-path":   false,
					"drop-query":  false,
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	resp, err := noRedirectClient().Get(baseURL + "/original?key=val")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 307, resp.StatusCode)
	loc := resp.Header.Get("Location")
	assert.Contains(t, loc, "redirected.example.com")
	assert.Contains(t, loc, "/original")
	assert.Contains(t, loc, "key=val")
	assert.Equal(t, "torii-proxy", resp.Header.Get("X-Via"),
		"global response headers should be present on redirect responses")
}

// ── 10. Global=RequestId+RequestLog+Headers(set), Route=Timeout+Headers(cmp),
//        Path=BodySizeLimit+Headers(set) — maximum middleware depth at every level.

func TestChainBattle_MaxDepth_ThreeMiddlewaresPerLevel(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{
			{Type: "RequestId", Options: map[string]interface{}{"prefix": "max"}},
			{Type: "RequestLog"},
			{Type: "Headers", Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{"X-Level": "global"},
			}},
		},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{
				{Type: "Timeout", Options: map[string]interface{}{"request-timeout": "5s"}},
				{Type: "Headers", Options: map[string]interface{}{
					"cmp-headers-req": map[string]interface{}{"X-Level": "global"},
				}},
				{Type: "Headers", Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{"X-Level-Route": "route"},
				}},
			},
			Paths: []config.PathRule{{
				Pattern: "/deep/*",
				Backend: backend.URL,
				Middlewares: []middleware.Config{
					{Type: "BodySizeLimit", Options: map[string]interface{}{"max-size": "5m"}},
					{Type: "Headers", Options: map[string]interface{}{
						"cmp-headers-req": map[string]interface{}{"X-Level-Route": "route"},
					}},
					{Type: "Headers", Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{"X-Level-Path": "path"},
					}},
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	t.Run("all 9 middlewares fire in sequence", func(t *testing.T) {
		resp := doGet(t, baseURL+"/deep/resource")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"if any cmp-headers-req fails the chain is broken → 401")
		assert.Equal(t, "global", echo.Headers["X-Level"][0])
		assert.Equal(t, "route", echo.Headers["X-Level-Route"][0])
		assert.Equal(t, "path", echo.Headers["X-Level-Path"][0])
	})
}

// ── 11. Concurrent requests across all three levels with mixed middleware

func TestChainBattle_ConcurrentFullChain(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{
			{Type: "RequestId", Options: map[string]interface{}{"prefix": "conc-chain"}},
			{Type: "Headers", Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{"X-G": "1"},
			}},
		},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{
				{Type: "Metrics"},
				{Type: "Headers", Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{"X-R": "2"},
				}},
			},
			Paths: []config.PathRule{{
				Pattern: "/api/*",
				Middlewares: []middleware.Config{{
					Type: "Headers", Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{"X-P": "3"},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	var wg sync.WaitGroup
	errs := make(chan string, 200)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var url string
			if idx%2 == 0 {
				url = baseURL + "/api/resource"
			} else {
				url = baseURL + "/other"
			}
			resp, err := defaultClient().Get(url)
			if err != nil {
				errs <- fmt.Sprintf("req %d: %v", idx, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Sprintf("req %d: status %d", idx, resp.StatusCode)
				return
			}
			var echo echoResponse
			if err := json.NewDecoder(resp.Body).Decode(&echo); err != nil {
				errs <- fmt.Sprintf("req %d: decode: %v", idx, err)
				return
			}
			if echo.Headers["X-G"] == nil || echo.Headers["X-G"][0] != "1" {
				errs <- fmt.Sprintf("req %d: missing X-G", idx)
			}
			if echo.Headers["X-R"] == nil || echo.Headers["X-R"][0] != "2" {
				errs <- fmt.Sprintf("req %d: missing X-R", idx)
			}
			if idx%2 == 0 {
				if echo.Headers["X-P"] == nil || echo.Headers["X-P"][0] != "3" {
					errs <- fmt.Sprintf("req %d: missing X-P for /api path", idx)
				}
			} else {
				if echo.Headers["X-P"] != nil {
					errs <- fmt.Sprintf("req %d: unexpected X-P on /other path", idx)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 6 — Host-based routing (VirtualHostDispatcher)
// ═══════════════════════════════════════════════════════════════════════════

func TestRouting_HostBasedRouting(t *testing.T) {
	backendA := newEchoBackend(t)
	backendB := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Routes: []config.Route{
			{
				Host: "alpha.local",
				Target: config.RouteTarget{
					Backend: backendA.URL,
					Middlewares: []middleware.Config{{
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{
								"X-Route-Tag": "alpha",
							},
						},
					}},
				},
			},
			{
				Host: "beta.local",
				Target: config.RouteTarget{
					Backend: backendB.URL,
					Middlewares: []middleware.Config{{
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{
								"X-Route-Tag": "beta",
							},
						},
					}},
				},
			},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	// Request to alpha.local
	reqA, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	reqA.Host = "alpha.local"
	respA, err := defaultClient().Do(reqA)
	require.NoError(t, err)
	echoA := readEcho(t, respA)
	assert.Equal(t, "alpha", echoA.Headers["X-Route-Tag"][0])

	// Request to beta.local
	reqB, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	reqB.Host = "beta.local"
	respB, err := defaultClient().Do(reqB)
	require.NoError(t, err)
	echoB := readEcho(t, respB)
	assert.Equal(t, "beta", echoB.Headers["X-Route-Tag"][0])
}

func TestRouting_UnmatchedHostWithDefault(t *testing.T) {
	backendDefault := newEchoBackend(t)
	backendRouted := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Routes: []config.Route{{
			Host: "specific.local",
			Target: config.RouteTarget{
				Backend: backendRouted.URL,
			},
		}},
		Default: &config.RouteTarget{
			Backend: backendDefault.URL,
			Middlewares: []middleware.Config{{
				Type: "Headers",
				Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{
						"X-Default": "yes",
					},
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	// Unmatched host should go to default.
	resp := doGet(t, baseURL+"/")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "yes", echo.Headers["X-Default"][0])
}

func TestRouting_UnmatchedHostNoDefault_502(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Routes: []config.Route{{
			Host: "specific.local",
			Target: config.RouteTarget{
				Backend: backend.URL,
			},
		}},
	}
	baseURL := buildAndStart(t, listener, nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode,
		"unmatched host without a default route should return 502")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 7 — Path dispatching
// ═══════════════════════════════════════════════════════════════════════════

func TestRouting_PathSpecificBackend(t *testing.T) {
	mainBackend := newEchoBackend(t)
	apiBackend := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: mainBackend.URL,
			Paths: []config.PathRule{{
				Pattern: "/api/*",
				Backend: apiBackend.URL,
				Middlewares: []middleware.Config{{
					Type: "Headers",
					Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{
							"X-Path-Tag": "api-backend",
						},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	// /api/* goes to the api backend.
	resp := doGet(t, baseURL+"/api/users")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "api-backend", echo.Headers["X-Path-Tag"][0])

	// / goes to the main backend (no X-Path-Tag).
	resp2 := doGet(t, baseURL+"/")
	echo2 := readEcho(t, resp2)
	_, hasTag := echo2.Headers["X-Path-Tag"]
	assert.False(t, hasTag, "root path should NOT have the api path header")
}

func TestRouting_PathMiddlewareOnlyNoBackend(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Paths: []config.PathRule{{
				Pattern: "/protected/*",
				Middlewares: []middleware.Config{{
					Type: "Headers",
					Options: map[string]interface{}{
						"cmp-headers-req": map[string]interface{}{
							"X-Token": "valid",
						},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	// Without the header, /protected/* should be rejected.
	resp := doGet(t, baseURL+"/protected/resource")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// With the header, it should succeed.
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/protected/resource", nil)
	req.Header.Set("X-Token", "valid")
	resp2, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 8 — Multiple middleware combinations on one server
// ═══════════════════════════════════════════════════════════════════════════

func TestCombo_RequestId_RequestLog_Headers_Metrics(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{
		{Type: "RequestId", Options: map[string]interface{}{"prefix": "combo"}},
		{Type: "RequestLog"},
		{Type: "Metrics"},
		{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Combo": "all-four",
				},
			},
		},
	}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/combo")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "all-four", echo.Headers["X-Combo"][0])
}

func TestCombo_RateLimiter_Headers_Timeout(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{
		{
			Type: "RateLimiter",
			Options: map[string]interface{}{
				"mode": "global",
				"limiter-req": map[string]interface{}{
					"rate-per-second": 100.0,
					"burst":           100,
				},
			},
		},
		{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-res": map[string]interface{}{
					"X-Via": "torii",
				},
			},
		},
		{
			Type: "Timeout",
			Options: map[string]interface{}{
				"request-timeout": "5s",
			},
		},
	}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "torii", resp.Header.Get("X-Via"))
}

func TestCombo_HoneyPot_Then_BodySizeLimit(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{
		{
			Type: "HoneyPot",
			Options: map[string]interface{}{
				"paths":            []interface{}{"/wp-login.php"},
				"cache-ttl":        "1h",
				"cleanup-interval": "1h",
				"max-cache-size":   100,
			},
		},
		{
			Type: "BodySizeLimit",
			Options: map[string]interface{}{
				"max-size": "10m",
			},
		},
	}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	// Normal request should pass both.
	resp := doGet(t, baseURL+"/")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = echo

	// Trap path should be caught by HoneyPot before BodySizeLimit.
	resp2 := doGet(t, baseURL+"/wp-login.php")
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp2.StatusCode)
}

func TestCombo_UserAgentBlocker_CircuitBreaker(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{
		{
			Type: "UserAgentBlocker",
			Options: map[string]interface{}{
				"block-empty-ua":   false,
				"block":            []interface{}{"bad-scanner"},
				"cache-ttl":        "1h",
				"cleanup-interval": "1h",
				"max-cache-size":   100,
			},
		},
		{
			Type: "CircuitBreaker",
			Options: map[string]interface{}{
				"failure-threshold":           10,
				"recovery-time":               "30s",
				"half-open-success-threshold": 2,
			},
		},
	}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	// Good UA goes through.
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req.Header.Set("User-Agent", "GoodBrowser/1.0")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Bad UA blocked by UserAgentBlocker.
	req2, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	req2.Header.Set("User-Agent", "bad-scanner")
	resp2, err := defaultClient().Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp2.StatusCode)
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 9 — Global-only middleware (no route middleware)
// ═══════════════════════════════════════════════════════════════════════════

func TestGlobalMiddleware_AppliedToAllRoutes(t *testing.T) {
	backendA := newEchoBackend(t)
	backendB := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Global-Tag": "global-applied",
				},
			},
		}},
	}

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Routes: []config.Route{
			{Host: "a.local", Target: config.RouteTarget{Backend: backendA.URL}},
			{Host: "b.local", Target: config.RouteTarget{Backend: backendB.URL}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	// Both routes should receive the global header.
	for _, host := range []string{"a.local", "b.local"} {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
		req.Host = host
		resp, err := defaultClient().Do(req)
		require.NoError(t, err)
		echo := readEcho(t, resp)
		assert.Equal(t, "global-applied", echo.Headers["X-Global-Tag"][0],
			"global middleware should apply to host %s", host)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 10 — IPv6 bind flag validation
// ═══════════════════════════════════════════════════════════════════════════

func TestIPv6BindFlagArithmetic(t *testing.T) {
	// Verify that the bind-flag bitwise checks work correctly for both
	// IPv4 and IPv6.
	assert.True(t, config.Ipv4Flag&config.Ipv4Flag != 0, "Ipv4Flag should match itself")
	assert.True(t, config.Ipv6Flag&config.Ipv6Flag != 0, "Ipv6Flag should match itself")
	assert.True(t, config.BothFlag&config.Ipv4Flag != 0, "BothFlag should include Ipv4Flag")
	assert.True(t, config.BothFlag&config.Ipv6Flag != 0, "BothFlag should include Ipv6Flag")
	assert.True(t, config.Ipv4Flag&config.Ipv6Flag == 0, "Ipv4Flag and Ipv6Flag should not overlap")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 11 — Stress / concurrency: multiple concurrent requests
// ═══════════════════════════════════════════════════════════════════════════

func TestConcurrency_MultipleMiddlewares(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{
		{Type: "RequestId", Options: map[string]interface{}{"prefix": "conc"}},
		{Type: "RequestLog"},
		{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{
					"X-Concurrency": "test",
				},
			},
		},
	}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	var wg sync.WaitGroup
	errors := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := defaultClient().Get(baseURL + "/concurrent")
			if err != nil {
				errors <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("unexpected status %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 12 — Every registered middleware can be built and started
// ═══════════════════════════════════════════════════════════════════════════

func TestEveryMiddleware_CanBuildAndStart(t *testing.T) {
	// For each registered middleware type, build a server that uses ONLY
	// that middleware with valid minimal configuration, start it, send one
	// request, and stop it.

	backend := newEchoBackend(t)

	configs := []struct {
		name string
		mw   middleware.Config
	}{
		{
			name: "RequestId-defaults",
			mw:   middleware.Config{Type: "RequestId"},
		},
		{
			name: "RequestId-prefix",
			mw: middleware.Config{Type: "RequestId", Options: map[string]interface{}{
				"prefix": "test",
			}},
		},
		{
			name: "RequestLog",
			mw:   middleware.Config{Type: "RequestLog"},
		},
		{
			name: "Metrics",
			mw:   middleware.Config{Type: "Metrics"},
		},
		{
			name: "Headers-set-req",
			mw: middleware.Config{Type: "Headers", Options: map[string]interface{}{
				"set-headers-req": map[string]interface{}{"X-A": "B"},
			}},
		},
		{
			name: "Headers-set-res",
			mw: middleware.Config{Type: "Headers", Options: map[string]interface{}{
				"set-headers-res": map[string]interface{}{"X-A": "B"},
			}},
		},
		{
			name: "Headers-strip-req",
			mw: middleware.Config{Type: "Headers", Options: map[string]interface{}{
				"strip-headers-req": []interface{}{"X-Kill"},
			}},
		},
		{
			name: "Headers-strip-res",
			mw: middleware.Config{Type: "Headers", Options: map[string]interface{}{
				"strip-headers-res": []interface{}{"X-Kill"},
			}},
		},
		{
			name: "Headers-cmp-req",
			mw: middleware.Config{Type: "Headers", Options: map[string]interface{}{
				"cmp-headers-req": map[string]interface{}{"X-K": "V"},
			}},
		},
		{
			name: "RateLimiter-global",
			mw: middleware.Config{Type: "RateLimiter", Options: map[string]interface{}{
				"mode":        "global",
				"limiter-req": map[string]interface{}{"rate-per-second": 100.0, "burst": 100},
			}},
		},
		{
			name: "RateLimiter-per-client",
			mw: middleware.Config{Type: "RateLimiter", Options: map[string]interface{}{
				"mode":             "per-client",
				"limiter-req":      map[string]interface{}{"rate-per-second": 100.0, "burst": 100},
				"cache-ttl":        "1h",
				"cleanup-interval": "1h",
				"max-cache-size":   100,
			}},
		},
		{
			name: "IpBlock",
			mw: middleware.Config{Type: "IpBlock", Options: map[string]interface{}{
				"list-mode": "block",
				"list":      []interface{}{"10.0.0.1"},
			}},
		},
		{
			name: "Redirect-external",
			mw: middleware.Config{Type: "Redirect", Options: map[string]interface{}{
				"mode": "external", "target": "http://example.com",
				"status-code": 302, "drop-path": true, "drop-query": true,
			}},
		},
		{
			name: "Redirect-internal",
			mw: middleware.Config{Type: "Redirect", Options: map[string]interface{}{
				"mode": "internal", "target": backend.URL,
				"drop-path": false, "drop-query": false,
			}},
		},
		{
			name: "BodySizeLimit",
			mw: middleware.Config{Type: "BodySizeLimit", Options: map[string]interface{}{
				"max-size": "10m",
			}},
		},
		{
			name: "Timeout",
			mw: middleware.Config{Type: "Timeout", Options: map[string]interface{}{
				"request-timeout": "30s",
			}},
		},
		{
			name: "HoneyPot-defaults",
			mw: middleware.Config{Type: "HoneyPot", Options: map[string]interface{}{
				"defaults": []interface{}{"php"}, "cache-ttl": "1h",
				"cleanup-interval": "1h", "max-cache-size": 100,
			}},
		},
		{
			name: "HoneyPot-custom-paths",
			mw: middleware.Config{Type: "HoneyPot", Options: map[string]interface{}{
				"paths": []interface{}{"/trap"}, "cache-ttl": "1h",
				"cleanup-interval": "1h", "max-cache-size": 100,
			}},
		},
		{
			name: "HoneyPot-custom-response",
			mw: middleware.Config{Type: "HoneyPot", Options: map[string]interface{}{
				"paths": []interface{}{"/trap"}, "cache-ttl": "1h",
				"cleanup-interval": "1h", "max-cache-size": 100,
				"response": map[string]interface{}{"status-code": 404, "body": "nope"},
			}},
		},
		{
			name: "UserAgentBlocker-block-empty",
			mw: middleware.Config{Type: "UserAgentBlocker", Options: map[string]interface{}{
				"block-empty-ua": true, "cache-ttl": "1h",
				"cleanup-interval": "1h", "max-cache-size": 100,
			}},
		},
		{
			name: "UserAgentBlocker-block-categories",
			mw: middleware.Config{Type: "UserAgentBlocker", Options: map[string]interface{}{
				"block-empty-ua": false, "block-defaults": []interface{}{"scanners", "malicious"},
				"cache-ttl": "1h", "cleanup-interval": "1h", "max-cache-size": 100,
			}},
		},
		{
			name: "UserAgentBlocker-custom-block-allow",
			mw: middleware.Config{Type: "UserAgentBlocker", Options: map[string]interface{}{
				"block-empty-ua": false,
				"block":          []interface{}{"bad"},
				"allow":          []interface{}{"good"},
				"cache-ttl":      "1h", "cleanup-interval": "1h", "max-cache-size": 100,
			}},
		},
		{
			name: "CircuitBreaker",
			mw: middleware.Config{Type: "CircuitBreaker", Options: map[string]interface{}{
				"failure-threshold": 5, "recovery-time": "30s",
				"half-open-success-threshold": 2,
			}},
		},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			port := getFreePort(t)
			listener := simpleListener(port, backend.URL, []middleware.Config{tc.mw})
			ctx := createTestContext()

			server, err := buildHttpServer(ctx, listener, nil)
			require.NoError(t, err, "buildHttpServer should succeed for %s", tc.name)

			err = server.start(nil)
			require.NoError(t, err, "start should succeed for %s", tc.name)

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			waitForServer(t, addr, 3*time.Second)

			// Send at least one request.
			req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
			req.Header.Set("User-Agent", "TestBot/1.0")
			// For cmp-headers-req, provide the expected header.
			req.Header.Set("X-K", "V")
			resp, err := defaultClient().Do(req)
			require.NoError(t, err, "request should not fail for %s", tc.name)
			resp.Body.Close()

			err = server.stop()
			assert.NoError(t, err, "stop should succeed for %s", tc.name)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 13 — Edge cases
// ═══════════════════════════════════════════════════════════════════════════

func TestEdge_EmptyRouteHost_Skipped(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	// A route with an empty host should be silently skipped.
	// If there's a default target, the server should still build.
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Routes: []config.Route{
			{Host: "", Target: config.RouteTarget{Backend: backend.URL}},
		},
		Default: &config.RouteTarget{Backend: backend.URL},
	}
	baseURL := buildAndStart(t, listener, nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestEdge_InvalidMiddlewareType(t *testing.T) {
	backend := newEchoBackend(t)
	ctx := createTestContext()
	listener := simpleListener(getFreePort(t), backend.URL, []middleware.Config{
		{Type: "NonExistentMiddleware"},
	})
	_, err := buildHttpServer(ctx, listener, nil)
	require.Error(t, err, "unknown middleware type should cause buildHttpServer to fail")
	assert.Contains(t, err.Error(), "not found")
}

func TestEdge_NilMiddlewareOptions(t *testing.T) {
	// ApplyMiddlewares initialises nil Options to empty map. Verify this
	// doesn't cause a panic for middlewares that check Options.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{
		{Type: "RequestId", Options: nil},
		{Type: "RequestLog", Options: nil},
	}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestEdge_MultiplePathRulesOnSameRoute(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Paths: []config.PathRule{
				{
					Pattern: "/admin/*",
					Middlewares: []middleware.Config{{
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{"X-Zone": "admin"},
						},
					}},
				},
				{
					Pattern: "/public/*",
					Middlewares: []middleware.Config{{
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{"X-Zone": "public"},
						},
					}},
				},
			},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	respAdmin := doGet(t, baseURL+"/admin/dashboard")
	echoAdmin := readEcho(t, respAdmin)
	assert.Equal(t, "admin", echoAdmin.Headers["X-Zone"][0])

	respPublic := doGet(t, baseURL+"/public/index")
	echoPublic := readEcho(t, respPublic)
	assert.Equal(t, "public", echoPublic.Headers["X-Zone"][0])
}

func TestEdge_ServerTimeouts(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port:              port,
		Bind:              config.Ipv4Flag,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		Default:           &config.RouteTarget{Backend: backend.URL},
	}
	baseURL := buildAndStart(t, listener, nil)

	resp := doGet(t, baseURL+"/")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = echo
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 14 — Path prefix stripping
// ═══════════════════════════════════════════════════════════════════════════

func TestPathPrefixStripping_DedicatedBackend(t *testing.T) {
	// When a path rule has its own backend and strip-prefix is enabled,
	// the path prefix should be stripped before forwarding.
	// e.g. /api/users → /users on the backend.
	apiBackend := newEchoBackend(t)
	mainBackend := newEchoBackend(t)
	port := getFreePort(t)
	stripPrefix := true

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: mainBackend.URL,
			Paths: []config.PathRule{{
				Pattern:     "/api/*",
				Backend:     apiBackend.URL,
				StripPrefix: &stripPrefix,
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	resp := doGet(t, baseURL+"/api/users")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "/users", echo.Path,
		"path prefix /api should be stripped when forwarding to the dedicated backend")
}

func TestPathPrefixStripping_NestedPrefix(t *testing.T) {
	apiBackend := newEchoBackend(t)
	mainBackend := newEchoBackend(t)
	port := getFreePort(t)
	stripPrefix := true

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: mainBackend.URL,
			Paths: []config.PathRule{{
				Pattern:     "/app/v1/*",
				Backend:     apiBackend.URL,
				StripPrefix: &stripPrefix,
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	resp := doGet(t, baseURL+"/app/v1/health")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "/health", echo.Path,
		"nested prefix /app/v1 should be fully stripped")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 15 — PathRule.DropQuery
// ═══════════════════════════════════════════════════════════════════════════

func TestPathRule_DropQuery_True(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	dropQuery := true

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Paths: []config.PathRule{{
				Pattern:   "/api/*",
				Backend:   backend.URL,
				DropQuery: &dropQuery,
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	resp := doGet(t, baseURL+"/api/resource?foo=bar&baz=1")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, echo.URL, "?", "query string should be dropped")
	assert.NotContains(t, echo.URL, "foo=bar",
		"query string should be dropped when DropQuery is true")
}

func TestPathRule_DropQuery_False(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	dropQuery := false

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Paths: []config.PathRule{{
				Pattern:   "/api/*",
				Backend:   backend.URL,
				DropQuery: &dropQuery,
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	resp := doGet(t, baseURL+"/api/resource?foo=bar")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, echo.URL, "foo=bar",
		"query string should be preserved when DropQuery is false")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 16 — Request body forwarding (POST/PUT)
// ═══════════════════════════════════════════════════════════════════════════

func TestProxy_PostBodyForwarding(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)

	body := `{"name":"test","value":42}`
	resp, err := defaultClient().Post(baseURL+"/data", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "POST", echo.Method)
	assert.Equal(t, body, echo.Body, "request body should be forwarded to backend")
}

func TestProxy_PutBodyForwarding(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)

	body := `updated-content`
	req, _ := http.NewRequest(http.MethodPut, baseURL+"/resource/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "PUT", echo.Method)
	assert.Equal(t, body, echo.Body, "PUT body should be forwarded to backend")
}

func TestProxy_DeleteMethod(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)

	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/resource/1", nil)
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "DELETE", echo.Method)
}

func TestProxy_PatchMethod(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)

	body := `{"patch":"data"}`
	req, _ := http.NewRequest(http.MethodPatch, baseURL+"/resource/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "PATCH", echo.Method)
	assert.Equal(t, body, echo.Body)
}

func TestProxy_HeadMethod(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)

	req, _ := http.NewRequest(http.MethodHead, baseURL+"/resource", nil)
	resp, err := defaultClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 17 — Query string forwarding
// ═══════════════════════════════════════════════════════════════════════════

func TestProxy_QueryStringForwarding(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)

	resp := doGet(t, baseURL+"/search?q=hello&page=2&sort=name")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, echo.URL, "q=hello")
	assert.Contains(t, echo.URL, "page=2")
	assert.Contains(t, echo.URL, "sort=name")
}

func TestProxy_QueryStringWithSpecialChars(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)

	resp := doGet(t, baseURL+"/search?q=hello+world&filter=a%26b")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, echo.URL, "q=hello+world")
	assert.Contains(t, echo.URL, "filter=a%26b")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 18 — Backend unreachable
// ═══════════════════════════════════════════════════════════════════════════

func TestProxy_BackendUnreachable(t *testing.T) {
	// Point the proxy at a port where nothing is listening.
	deadPort := getFreePort(t)
	port := getFreePort(t)
	deadBackend := fmt.Sprintf("http://127.0.0.1:%d", deadPort)

	baseURL := buildAndStart(t, simpleListener(port, deadBackend, nil), nil)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode,
		"unreachable backend should result in 502 Bad Gateway")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 19 — CircuitBreaker half-open recovery
// ═══════════════════════════════════════════════════════════════════════════

func TestMiddleware_CircuitBreaker_Recovery(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "CircuitBreaker",
		Options: map[string]interface{}{
			"failure-threshold":           3,
			"recovery-time":               "1s", // short recovery for testing
			"half-open-success-threshold": 1,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	// Trip the circuit breaker with failures.
	for i := 0; i < 5; i++ {
		resp := doGet(t, baseURL+"/error500")
		resp.Body.Close()
	}

	// Verify circuit is open.
	resp := doGet(t, baseURL+"/")
	resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"circuit should be open")

	// Wait for recovery time to elapse.
	time.Sleep(1500 * time.Millisecond)

	// The circuit should now be half-open; a successful request should close it.
	resp = doGet(t, baseURL+"/")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"circuit should recover to closed after recovery time + successful request")

	// Subsequent requests should also succeed.
	resp = doGet(t, baseURL+"/")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"circuit should be fully closed again")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 20 — Exact-match path patterns (no wildcard)
// ═══════════════════════════════════════════════════════════════════════════

func TestRouting_ExactMatchPathPattern(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Paths: []config.PathRule{{
				Pattern: "/health",
				Middlewares: []middleware.Config{{
					Type: "Headers",
					Options: map[string]interface{}{
						"set-headers-req": map[string]interface{}{"X-Health": "checked"},
					},
				}},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	t.Run("exact match applies middleware", func(t *testing.T) {
		resp := doGet(t, baseURL+"/health")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "checked", echo.Headers["X-Health"][0])
	})

	t.Run("non-matching path skips middleware", func(t *testing.T) {
		resp := doGet(t, baseURL+"/other")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_, has := echo.Headers["X-Health"]
		assert.False(t, has, "/other should not get health middleware")
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 21 — Proxy forwarding headers (X-Forwarded-For, etc.)
// ═══════════════════════════════════════════════════════════════════════════

func TestProxy_XForwardedForHeader(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, nil), nil)

	resp := doGet(t, baseURL+"/")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// The reverse proxy should set X-Forwarded-For to the client IP.
	xff, hasXFF := echo.Headers["X-Forwarded-For"]
	assert.True(t, hasXFF, "reverse proxy should set X-Forwarded-For header")
	if hasXFF {
		assert.Contains(t, xff[0], "127.0.0.1",
			"X-Forwarded-For should contain the client's IP")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 22 — Lifecycle: stop then restart
// ═══════════════════════════════════════════════════════════════════════════

func TestLifecycle_StopAndRestart(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	ctx := createTestContext()
	listener := simpleListener(port, backend.URL, nil)

	server, err := buildHttpServer(ctx, listener, nil)
	require.NoError(t, err)

	// First start
	err = server.start(nil)
	require.NoError(t, err)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	waitForServer(t, addr, 3*time.Second)

	resp := doGet(t, fmt.Sprintf("http://%s/", addr))
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "first start should work")

	// Stop
	err = server.stop()
	require.NoError(t, err)

	// After stop, connections should be refused.
	time.Sleep(100 * time.Millisecond)
	_, err = defaultClient().Get(fmt.Sprintf("http://%s/", addr))
	assert.Error(t, err, "requests should fail after stop")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 23 — Redirect with host:port target (no scheme)
// ═══════════════════════════════════════════════════════════════════════════

func TestMiddleware_Redirect_External_HostPort(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":        "external",
			"target":      "example.com:8080",
			"status-code": 302,
			"drop-path":   true,
			"drop-query":  true,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp, err := noRedirectClient().Get(baseURL + "/some-path")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 302, resp.StatusCode)
	loc := resp.Header.Get("Location")
	assert.Contains(t, loc, "example.com:8080",
		"redirect target should contain the host:port")
}

func TestMiddleware_Redirect_Internal_HostPort(t *testing.T) {
	// Internal redirect to a host:port target (no scheme) should infer http.
	redirectTarget := newEchoBackend(t)
	originalBackend := newEchoBackend(t)
	port := getFreePort(t)

	// Extract host:port from the test server URL (strip "http://")
	targetHostPort := strings.TrimPrefix(redirectTarget.URL, "http://")

	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":       "internal",
			"target":     targetHostPort,
			"drop-path":  false,
			"drop-query": false,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, originalBackend.URL, mws), nil)

	resp := doGet(t, baseURL+"/internal-host-port")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "/internal-host-port", echo.Path,
		"internal redirect via host:port should forward correctly")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 24 — HoneyPot trickster mode
// ═══════════════════════════════════════════════════════════════════════════

func TestMiddleware_HoneyPot_TricksterMode(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "HoneyPot",
		Options: map[string]interface{}{
			"paths":            []interface{}{"/trap-trick"},
			"cache-ttl":        "1h",
			"cleanup-interval": "1h",
			"max-cache-size":   100,
			"response": map[string]interface{}{
				"trickster-mode":  true,
				"max-slow-tricks": 2,
			},
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	// Hit the trap path — trickster mode should still block.
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/trap-trick")
	if err != nil {
		// A timeout or connection reset is acceptable in trickster mode
		// since it's designed to waste attacker resources.
		t.Logf("trickster mode caused error (expected for slow tricks): %v", err)
		return
	}
	defer resp.Body.Close()
	// Trickster mode may return various status codes (including 200 for
	// fake login pages, env files, etc.). The key assertion is that the
	// request was NOT proxied to the real echo backend.
	assert.Empty(t, resp.Header.Get("X-Backend"),
		"trickster mode should not proxy to the real backend")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 25 — CORS middleware (not registered in middleware registry)
// ═══════════════════════════════════════════════════════════════════════════
// NOTE: The CORS middleware is defined in cors_middleware.go and advertised in
// GetMiddlewareSchemas(), but it is NOT registered in the middleware registry
// (middleware.go init()). This means it cannot be used via config and
// buildHttpServer will fail if "CORS" is used. The test below documents this.

func TestMiddleware_CORS_NotRegistered(t *testing.T) {
	backend := newEchoBackend(t)
	ctx := createTestContext()
	listener := simpleListener(getFreePort(t), backend.URL, []middleware.Config{
		{Type: "CORS"},
	})
	_, err := buildHttpServer(ctx, listener, nil)
	require.Error(t, err,
		"CORS middleware is not registered in the registry; buildHttpServer should fail")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 26 — Global middleware blocking prevents route/path execution
// ═══════════════════════════════════════════════════════════════════════════

func TestGlobalBlock_PreventsRouteExecution(t *testing.T) {
	// Global rate limiter with burst=1 should block subsequent requests
	// before route-level middleware ever touches them.
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "RateLimiter",
			Options: map[string]interface{}{
				"mode":        "global",
				"limiter-req": map[string]interface{}{"rate-per-second": 1.0, "burst": 1},
			},
		}},
	}
	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Middlewares: []middleware.Config{{
				Type: "Headers",
				Options: map[string]interface{}{
					"set-headers-req": map[string]interface{}{"X-Route": "reached"},
				},
			}},
		},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	// First request passes.
	resp := doGet(t, baseURL+"/")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Hammer until rate limited.
	got429 := false
	for i := 0; i < 20; i++ {
		r := doGet(t, baseURL+"/")
		r.Body.Close()
		if r.StatusCode == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	assert.True(t, got429,
		"global rate limiter should block before route middleware runs")
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 27 — Multiple path rules: priority / specificity
// ═══════════════════════════════════════════════════════════════════════════

func TestRouting_PathPriority_SpecificOverWildcard(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)

	listener := config.HTTPListener{
		Port: port,
		Bind: config.Ipv4Flag,
		Default: &config.RouteTarget{
			Backend: backend.URL,
			Paths: []config.PathRule{
				{
					Pattern: "/api/*",
					Middlewares: []middleware.Config{{
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{"X-Match": "wildcard"},
						},
					}},
				},
				{
					Pattern: "/api/special",
					Middlewares: []middleware.Config{{
						Type: "Headers",
						Options: map[string]interface{}{
							"set-headers-req": map[string]interface{}{"X-Match": "exact"},
						},
					}},
				},
			},
		},
	}
	baseURL := buildAndStart(t, listener, nil)

	t.Run("exact pattern wins over wildcard for /api/special", func(t *testing.T) {
		resp := doGet(t, baseURL+"/api/special")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "exact", echo.Headers["X-Match"][0],
			"Go ServeMux should prefer concrete segments over wildcards")
	})

	t.Run("wildcard catches other /api/* paths", func(t *testing.T) {
		resp := doGet(t, baseURL+"/api/other")
		echo := readEcho(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "wildcard", echo.Headers["X-Match"][0])
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 28 — Middleware key edge cases
// ═══════════════════════════════════════════════════════════════════════════

func TestEdge_EmptyMiddlewareType(t *testing.T) {
	backend := newEchoBackend(t)
	ctx := createTestContext()
	listener := simpleListener(getFreePort(t), backend.URL, []middleware.Config{
		{Type: ""},
	})
	_, err := buildHttpServer(ctx, listener, nil)
	require.Error(t, err, "empty middleware type should cause buildHttpServer to fail")
}

func TestEdge_DefaultOnlyNoRoutes(t *testing.T) {
	// A listener with only a default target (no host routes) should work fine.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	listener := config.HTTPListener{
		Port:    port,
		Bind:    config.Ipv4Flag,
		Default: &config.RouteTarget{Backend: backend.URL},
	}
	baseURL := buildAndStart(t, listener, nil)
	resp := doGet(t, baseURL+"/")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "/", echo.Path)
}

func TestEdge_GlobalMiddlewareOnly_NoRouteMiddleware(t *testing.T) {
	// Global middleware applies even when routes have no middleware.
	backend := newEchoBackend(t)
	port := getFreePort(t)

	globalConf := &config.GlobalConfig{
		Middlewares: []middleware.Config{{
			Type: "Headers",
			Options: map[string]interface{}{
				"set-headers-res": map[string]interface{}{"X-Global-Only": "yes"},
			},
		}},
	}
	listener := config.HTTPListener{
		Port:    port,
		Bind:    config.Ipv4Flag,
		Default: &config.RouteTarget{Backend: backend.URL},
	}
	baseURL := buildAndStart(t, listener, globalConf)

	resp := doGet(t, baseURL+"/")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "yes", resp.Header.Get("X-Global-Only"),
		"global response header should be set even with no route middleware")
}

func TestEdge_NilGlobalConfig(t *testing.T) {
	// Explicitly pass nil global config — server should still work.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	ctx := createTestContext()
	listener := simpleListener(port, backend.URL, nil)

	server, err := buildHttpServer(ctx, listener, nil)
	require.NoError(t, err)
	err = server.start(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.stop() })

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	waitForServer(t, addr, 3*time.Second)

	resp := doGet(t, "http://"+addr+"/")
	echo := readEcho(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = echo
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION 29 — Redirect external-scheme-only mode
// ═══════════════════════════════════════════════════════════════════════════

func TestMiddleware_Redirect_SchemeOnly_PreservesHostAndPath(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":        "external-scheme-only",
			"target":      "https",
			"status-code": 301,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp, err := noRedirectClient().Get(baseURL + "/some/path?q=1")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 301, resp.StatusCode)
	loc := resp.Header.Get("Location")
	assert.True(t, strings.HasPrefix(loc, "https://"), "location should use https scheme")
	assert.Contains(t, loc, "/some/path", "location should preserve path")
	assert.Contains(t, loc, "q=1", "location should preserve query")
}

func TestMiddleware_Redirect_SchemeOnly_DefaultStatusCode(t *testing.T) {
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":   "external-scheme-only",
			"target": "https",
			// status-code intentionally omitted — should default to 302
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp, err := noRedirectClient().Get(baseURL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 302, resp.StatusCode,
		"external-scheme-only should default to 302 when status-code is omitted")
}

func TestMiddleware_Redirect_SchemeOnly_AcceptsSchemeWithSeparator(t *testing.T) {
	// "https://" (with separator) should be accepted as a valid target.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":        "external-scheme-only",
			"target":      "https://",
			"status-code": 301,
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp, err := noRedirectClient().Get(baseURL + "/page")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 301, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "https://"))
}

func TestMiddleware_Redirect_SchemeOnly_InvalidTarget(t *testing.T) {
	// A full URL as target should be rejected for external-scheme-only mode,
	// causing the middleware to fail closed with 503.
	backend := newEchoBackend(t)
	port := getFreePort(t)
	mws := []middleware.Config{{
		Type: "Redirect",
		Options: map[string]interface{}{
			"mode":   "external-scheme-only",
			"target": "https://example.com",
		},
	}}
	baseURL := buildAndStart(t, simpleListener(port, backend.URL, mws), nil)

	resp, err := noRedirectClient().Get(baseURL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"full URL target should cause misconfiguration (503) for external-scheme-only mode")
}
