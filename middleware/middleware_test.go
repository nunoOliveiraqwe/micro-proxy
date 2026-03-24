package middleware

import (
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
	assert.Len(t, names, 3)
	assert.Contains(t, names, "Metrics")
	assert.Contains(t, names, "RequestId")
	assert.Contains(t, names, "RequestLog")
}

// ---------------------------------------------------------------------------
// ApplyMiddlewares
// ---------------------------------------------------------------------------

func TestApplyMiddlewares_NilHandler(t *testing.T) {
	_, err := ApplyMiddlewares(nil, []Config{{Type: "RequestId"}}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestApplyMiddlewares_UnknownMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	_, err := ApplyMiddlewares(handler, []Config{{Type: "Unknown"}}, nil)
	assert.Error(t, err)
}

func TestApplyMiddlewares_EmptyChain(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	result, err := ApplyMiddlewares(handler, []Config{}, nil)
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

	result, err := ApplyMiddlewares(handler, []Config{{Type: "RequestId"}}, nil)
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

	result, err := ApplyMiddlewares(handler, chain, nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	result.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}
