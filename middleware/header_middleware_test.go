package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	ctx2 "github.com/nunoOliveiraqwe/torii/middleware/ctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHeadersMiddleware_NoRulesReturnsNext(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	mw := HeadersMiddleware(context.Background(), next, Config{Options: map[string]interface{}{}})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHeadersMiddleware_SetRequestHeaderFromRequestResolver(t *testing.T) {
	const remoteAddr = "203.0.113.10:41234"
	called := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, remoteAddr, r.Header.Get("X-Client-Addr"))
		w.WriteHeader(http.StatusOK)
	})

	conf := Config{Options: map[string]interface{}{
		"set-headers-req": map[string]interface{}{
			"X-Client-Addr": "$remote_addr",
		},
	}}
	mw := ctx2.InjectContextStruct(HeadersMiddleware(context.Background(), next, conf))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHeadersMiddleware_SetRequestHeaderFromRequestResolverWithEnvVar(t *testing.T) {
	envVarValue := "env-value"
	envVarName := "TEST_ENV_VAR"
	t.Setenv(envVarName, envVarValue)
	called := false

	header := "X-Client-Secret"
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, envVarValue, r.Header.Get(header))
		w.WriteHeader(http.StatusOK)
	})

	conf := Config{Options: map[string]interface{}{
		"set-headers-req": map[string]interface{}{
			header: "$env:" + envVarName,
		},
	}}
	mw := ctx2.InjectContextStruct(HeadersMiddleware(context.Background(), next, conf))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHeadersMiddleware_CompareRequestHeaderRejectsMismatch(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	conf := Config{Options: map[string]interface{}{
		"cmp-headers-req": map[string]interface{}{
			"X-Token": "expected",
		},
	}}
	mw := ctx2.InjectContextStruct(HeadersMiddleware(context.Background(), next, conf))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Token", "actual")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHeadersMiddleware_CompareRequestHeaderDoesNotLogSecretValues(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	conf := Config{Options: map[string]interface{}{
		"cmp-headers-req": map[string]interface{}{
			"X-Token": "expected-secret",
		},
	}}
	mw := HeadersMiddleware(context.Background(), next, conf)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Token", "actual-secret")
	ctxStruct := ctx2.ContextStruct{Logger: logger}
	req = req.WithContext(context.WithValue(req.Context(), ctxkeys.ContextStruct, &ctxStruct))

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NotEmpty(t, logs.All())

	for _, entry := range logs.All() {
		assert.NotContains(t, entry.Message, "expected-secret")
		assert.NotContains(t, entry.Message, "actual-secret")
		for _, field := range entry.Context {
			assert.NotEqual(t, "expected-secret", field.String)
			assert.NotEqual(t, "actual-secret", field.String)
		}
	}
}

func TestHeadersMiddleware_SetAndStripResponseHeadersWhenHandlerDoesNotWrite(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("X-Remove-Me", "present")
	})

	conf := Config{Options: map[string]interface{}{
		"set-headers-res": map[string]interface{}{
			"X-Frame-Options": "DENY",
		},
		"strip-headers-res": []interface{}{"X-Remove-Me"},
	}}
	mw := HeadersMiddleware(context.Background(), next, conf)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	_, exists := rec.Header()["X-Remove-Me"]
	assert.True(t, exists)
	assert.Empty(t, rec.Header().Values("X-Remove-Me"))
}

func TestHeadersMiddleware_InvalidConfigurationFailsClosed(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	conf := Config{Options: map[string]interface{}{
		"cmp-headers-req": map[string]interface{}{
			"X-Token": "$unknown_request_var",
		},
	}}
	mw := HeadersMiddleware(context.Background(), next, conf)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "HeadersMiddleware misconfigured")
}

func TestHeadersMiddleware_InvalidOptionTypeFailsClosed(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	conf := Config{Options: map[string]interface{}{
		"set-headers-req": []interface{}{"X-Header"},
	}}
	mw := HeadersMiddleware(context.Background(), next, conf)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestCompileValueResolverRejectsUnknownDollarValue(t *testing.T) {
	resolver, err := compileValueResolver("$unknown_request_var")

	require.Error(t, err)
	assert.Nil(t, resolver)
	assert.Contains(t, err.Error(), "unknown request resolver")
}

func TestHeadersSchemaIncludesRequestResolverSuggestions(t *testing.T) {
	var headersSchema *MiddlewareSchema
	for _, schema := range GetMiddlewareSchemas() {
		if schema.Name == "Headers" {
			s := schema
			headersSchema = &s
			break
		}
	}
	require.NotNil(t, headersSchema)

	fieldsWithSuggestions := map[string]bool{
		"set-headers-req": false,
		"cmp-headers-req": false,
	}

	for _, field := range headersSchema.Fields {
		if _, ok := fieldsWithSuggestions[field.Key]; !ok {
			continue
		}

		for _, suggestion := range field.Suggestions {
			if suggestion.Value == "$remote_addr" {
				assert.NotEmpty(t, suggestion.Description)
				fieldsWithSuggestions[field.Key] = true
			}
		}
	}

	for field, found := range fieldsWithSuggestions {
		assert.True(t, found, "%s should include $remote_addr suggestion", field)
	}
}
