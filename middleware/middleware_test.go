package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func init() {
	zap.ReplaceGlobals(zap.NewNop())
}

// ---------------------------------------------------------------------------
// MiddlewareExists
// ---------------------------------------------------------------------------

func TestMiddlewareExists_KnownKeys(t *testing.T) {
	assert.True(t, MiddlewareExists("Metrics"))
	assert.True(t, MiddlewareExists("RequestId"))
	assert.True(t, MiddlewareExists("RequestLog"))
}

func TestMiddlewareExists_UnknownKey(t *testing.T) {
	assert.False(t, MiddlewareExists("NonExistent"))
}

func TestMiddlewareExists_EmptyKey(t *testing.T) {
	assert.False(t, MiddlewareExists(""))
}

// ---------------------------------------------------------------------------
// GetMiddleware
// ---------------------------------------------------------------------------

func TestGetMiddleware_Success(t *testing.T) {
	mw, err := GetMiddleware("Metrics")
	require.NoError(t, err)
	assert.NotNil(t, mw)
}

func TestGetMiddleware_NotFound(t *testing.T) {
	_, err := GetMiddleware("Unknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetMiddleware_EmptyKey(t *testing.T) {
	_, err := GetMiddleware("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// ---------------------------------------------------------------------------
// GetAvailableMiddlewares
// ---------------------------------------------------------------------------

func TestGetAvailableMiddlewares(t *testing.T) {
	names := GetAvailableMiddlewares()
	assert.Len(t, names, 14)
	assert.Contains(t, names, "Metrics")
	assert.Contains(t, names, "RequestId")
	assert.Contains(t, names, "RequestLog")
	assert.Contains(t, names, "Headers")
	assert.Contains(t, names, "RateLimiter")
	assert.Contains(t, names, "CountryBlock")
	assert.Contains(t, names, "IpBlock")
	assert.Contains(t, names, "Redirect")
	assert.Contains(t, names, "BodySizeLimit")
	assert.Contains(t, names, "Timeout")
	assert.Contains(t, names, "HoneyPot")
	assert.Contains(t, names, "UserAgentBlocker")
	assert.Contains(t, names, "CircuitBreaker")
}

// ---------------------------------------------------------------------------
// ApplyMiddlewares
// ---------------------------------------------------------------------------

func TestApplyMiddlewares_NilHandler(t *testing.T) {
	_, err := ApplyMiddlewares(context.Background(), nil, []Config{{Type: "RequestId"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestApplyMiddlewares_UnknownMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	_, err := ApplyMiddlewares(context.Background(), handler, []Config{{Type: "Unknown"}})
	assert.Error(t, err)
}

func TestApplyMiddlewares_EmptyChain(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	result, err := ApplyMiddlewares(context.Background(), handler, []Config{})
	require.NoError(t, err)
	assert.NotNil(t, result)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	result.ServeHTTP(rec, req)
	assert.True(t, called)
}

func TestApplyMiddlewares_SingleMiddleware(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	result, err := ApplyMiddlewares(context.Background(), handler, []Config{{Type: "RequestId"}})
	require.NoError(t, err)
	assert.NotNil(t, result)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	result.ServeHTTP(rec, req)
	assert.True(t, called)
}

func TestApplyMiddlewares_MultipleMiddlewares(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	chain := []Config{
		{Type: "RequestId"},
		{Type: "RequestLog"},
	}

	result, err := ApplyMiddlewares(context.Background(), handler, chain)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	result.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}
