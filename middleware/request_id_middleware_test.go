package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	ctx2 "github.com/nunoOliveiraqwe/torii/middleware/ctx"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// RequestIDMiddleware
// ---------------------------------------------------------------------------

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// The middleware should have set a request ID in the context.
		reqID := GetRequestIDFromRequest(r)
		assert.NotEmpty(t, reqID)
	})

	mw := ctx2.InjectContextStruct(RequestIDMiddleware(context.Background(), next, Config{}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.True(t, called)
}

func TestRequestIDMiddleware_UsesExistingID(t *testing.T) {
	prefix := "my-prefix"

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		reqID := GetRequestIDFromRequest(r)
		assert.Equal(t, prefix+"-42", reqID)
	})

	conf := Config{Options: map[string]interface{}{"prefix": prefix}}
	mw := ctx2.InjectContextStruct(RequestIDMiddleware(context.Background(), next, conf))

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	existingID := prefix + "-42"
	ctxStruct := ctx2.ContextStruct{RequestId: existingID}

	ctx := context.WithValue(req.Context(), ctxkeys.ContextStruct, &ctxStruct)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.True(t, called)
}

func TestRequestIDMiddleware_CompositeID_DifferentPrefix(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		reqID := GetRequestIDFromRequest(r)
		assert.Contains(t, reqID, "existing-request-id")
	})

	mw := ctx2.InjectContextStruct(RequestIDMiddleware(context.Background(), next, Config{}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctxStruct := ctx2.ContextStruct{RequestId: "existing-request-id"}
	ctx := context.WithValue(req.Context(), ctxkeys.ContextStruct, &ctxStruct)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.True(t, called)
}

func TestRequestIDMiddleware_WithPrefix(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		reqID := GetRequestIDFromRequest(r)
		assert.Contains(t, reqID, "testprefix")
	})

	conf := Config{
		Options: map[string]interface{}{
			"prefix": "testprefix",
		},
	}
	mw := ctx2.InjectContextStruct(RequestIDMiddleware(context.Background(), next, conf))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.True(t, called)
}

// ---------------------------------------------------------------------------
// GetRequestIDFromContext
// ---------------------------------------------------------------------------

func TestGetRequestIDFromContext_NilContext(t *testing.T) {
	result := GetRequestIDFromRequest(nil)
	assert.Empty(t, result)
}

func TestGetRequestIDFromContext_NoValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	result := GetRequestIDFromRequest(req)
	assert.Empty(t, result)
}

func TestGetRequestIDFromContext_WrongType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(context.Background(), ctxkeys.ContextStruct, 12345)
	req = req.WithContext(ctx)
	result := GetRequestIDFromRequest(req)
	assert.Empty(t, result)
}
