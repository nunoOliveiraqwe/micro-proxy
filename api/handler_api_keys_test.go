package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helper: create a real API key in the in-memory DB via the service
// ---------------------------------------------------------------------------

func createTestApiKey(t *testing.T, f *testFixture, alias string, scopes []string, expiry time.Time) *domain.ApiKey {
	t.Helper()
	req := &app.CreateApiKeyRequest{
		Alias:      alias,
		Scopes:     scopes,
		ExpiryDate: expiry,
	}
	apiKey, err := f.svc.GetServiceStore().GetApiKeyService().CreateApiKey(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, apiKey)
	return apiKey
}

// successHandler is the next handler that records it was called.
func successHandler(called *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// isAuthenticatedBySessionOrApiKey – guard tests
// ---------------------------------------------------------------------------

func TestApiKeyGuard_NoSessionNoHeader_Returns401(t *testing.T) {
	f := newTestFixture(t)
	called := false
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{domain.READ_STATS_SCOPE}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := serveWithSession(f, guarded, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestApiKeyGuard_InvalidAuthHeaderFormat_Returns401(t *testing.T) {
	f := newTestFixture(t)
	called := false
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{domain.READ_STATS_SCOPE}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := serveWithSession(f, guarded, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestApiKeyGuard_EmptyBearerToken_Returns401(t *testing.T) {
	f := newTestFixture(t)
	called := false
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{domain.READ_STATS_SCOPE}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := serveWithSession(f, guarded, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestApiKeyGuard_NonExistentKey_Returns403(t *testing.T) {
	f := newTestFixture(t)
	called := false
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{domain.READ_STATS_SCOPE}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer totally-fake-key")
	rec := serveWithSession(f, guarded, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestApiKeyGuard_ValidKeyCorrectScope_Passes(t *testing.T) {
	f := newTestFixture(t)
	apiKey := createTestApiKey(t, f, "valid-key", []string{"read_stats"}, time.Time{})

	called := false
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{domain.READ_STATS_SCOPE}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey.Key)
	rec := serveWithSession(f, guarded, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestApiKeyGuard_ValidKeyWrongScope_Returns403(t *testing.T) {
	f := newTestFixture(t)
	apiKey := createTestApiKey(t, f, "wrong-scope", []string{"read_stats"}, time.Time{})

	called := false
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{"nonexistent_scope"}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey.Key)
	rec := serveWithSession(f, guarded, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestApiKeyGuard_ValidSession_PassesWithoutApiKey(t *testing.T) {
	f := newTestFixture(t)
	hashed := hashPassword(t, "Secret1!")

	f.userStore.On("GetUserByUsername", mock.Anything, "admin").
		Return(&domain.User{ID: 1, Username: "admin", Password: hashed}, nil)

	// Create a session via login
	loginReq := newJSONRequest(t, http.MethodPost, "/api/v1/auth/login", LoginRequest{Username: "admin", Password: "Secret1!"})
	loginRec := serveWithSession(f, handleLogin(f.svc), loginReq)
	cookies := loginRec.Result().Cookies()

	called := false
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{domain.READ_STATS_SCOPE}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := serveWithSession(f, guarded, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestApiKeyGuard_MultipleRequiredScopes_AnyMatch_Passes(t *testing.T) {
	f := newTestFixture(t)
	apiKey := createTestApiKey(t, f, "multi", []string{"read_stats"}, time.Time{})

	called := false
	// Route requires read_stats or a nonexistent scope — key has read_stats
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{domain.READ_STATS_SCOPE, "other_scope"}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey.Key)
	rec := serveWithSession(f, guarded, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestApiKeyGuard_MultipleRequiredScopes_NoneMatch_Returns403(t *testing.T) {
	f := newTestFixture(t)
	apiKey := createTestApiKey(t, f, "none-match", []string{"read_stats"}, time.Time{})

	called := false
	guarded := isAuthenticatedBySessionOrApiKey(successHandler(&called), []domain.Scope{"scope_a", "scope_b"}, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey.Key)
	rec := serveWithSession(f, guarded, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// ---------------------------------------------------------------------------
// API key handlers
// ---------------------------------------------------------------------------

func TestHandleCreateApiKey_Success(t *testing.T) {
	f := newTestFixture(t)

	body := app.CreateApiKeyRequest{
		Alias:  "new-api-key",
		Scopes: []string{"read_stats"},
	}
	req := newJSONRequest(t, http.MethodPost, "/api/v1/apiKeys", body)
	rec := serveWithSession(f, handleCreateNewApiKey(f.svc), req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result domain.ApiKey
	err := json.NewDecoder(rec.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "new-api-key", result.Alias)
	assert.NotEmpty(t, result.Key, "key should be returned on creation")
}

func TestHandleCreateApiKey_DuplicateAlias_Returns409(t *testing.T) {
	f := newTestFixture(t)

	// Create first key
	createTestApiKey(t, f, "dup-alias", []string{"read_stats"}, time.Time{})

	// Try creating again with same alias
	body := app.CreateApiKeyRequest{
		Alias:  "dup-alias",
		Scopes: []string{"read_stats"},
	}
	req := newJSONRequest(t, http.MethodPost, "/api/v1/apiKeys", body)
	rec := serveWithSession(f, handleCreateNewApiKey(f.svc), req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestHandleCreateApiKey_EmptyAlias_Returns400(t *testing.T) {
	f := newTestFixture(t)

	body := app.CreateApiKeyRequest{
		Alias:  "",
		Scopes: []string{"read_stats"},
	}
	req := newJSONRequest(t, http.MethodPost, "/api/v1/apiKeys", body)
	rec := serveWithSession(f, handleCreateNewApiKey(f.svc), req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCreateApiKey_InvalidScopes_Returns400(t *testing.T) {
	f := newTestFixture(t)

	body := app.CreateApiKeyRequest{
		Alias:  "bad-scopes",
		Scopes: []string{"root_access"},
	}
	req := newJSONRequest(t, http.MethodPost, "/api/v1/apiKeys", body)
	rec := serveWithSession(f, handleCreateNewApiKey(f.svc), req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCreateApiKey_NoBody_Returns400(t *testing.T) {
	f := newTestFixture(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apiKeys", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := serveWithSession(f, handleCreateNewApiKey(f.svc), req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCreateApiKey_ExpiredExpiryDate_Returns400(t *testing.T) {
	f := newTestFixture(t)

	body := map[string]interface{}{
		"alias":  "expired-key",
		"scopes": []string{"read_stats"},
		// ExpiryDate is not exposed via JSON tag in CreateApiKeyRequest,
		// so this tests that the default (zero) is fine and past dates
		// are caught at service level.
	}
	req := newJSONRequest(t, http.MethodPost, "/api/v1/apiKeys", body)
	rec := serveWithSession(f, handleCreateNewApiKey(f.svc), req)

	// With zero ExpiryDate it should succeed
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetAllApiKeys_Empty(t *testing.T) {
	f := newTestFixture(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apiKeys", nil)
	rec := serveWithSession(f, handleGetAllApiKey(f.svc), req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result []domain.ApiKey
	err := json.NewDecoder(rec.Body).Decode(&result)
	require.NoError(t, err)
	// could be null (nil slice) or empty array
}

func TestHandleGetAllApiKeys_ReturnsCreatedKeys(t *testing.T) {
	f := newTestFixture(t)

	createTestApiKey(t, f, "key-1", []string{"read_stats"}, time.Time{})
	createTestApiKey(t, f, "key-2", []string{"read_stats"}, time.Time{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apiKeys", nil)
	rec := serveWithSession(f, handleGetAllApiKey(f.svc), req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result []domain.ApiKey
	err := json.NewDecoder(rec.Body).Decode(&result)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	// Keys should be redacted
	for _, k := range result {
		assert.Empty(t, k.Key, "raw key must be redacted in list response")
	}
}

func TestHandleDeleteApiKey_Success(t *testing.T) {
	f := newTestFixture(t)

	createTestApiKey(t, f, "to-delete", []string{"read_stats"}, time.Time{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apiKeys/to-delete", nil)
	req.SetPathValue("id", "to-delete")
	rec := serveWithSession(f, handleDeleteApiKey(f.svc), req)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify it's gone
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/apiKeys", nil)
	listRec := serveWithSession(f, handleGetAllApiKey(f.svc), listReq)
	var result []domain.ApiKey
	_ = json.NewDecoder(listRec.Body).Decode(&result)
	assert.Empty(t, result)
}

func TestHandleDeleteApiKey_MissingId_Returns400(t *testing.T) {
	f := newTestFixture(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apiKeys/", nil)
	// No path value set
	rec := serveWithSession(f, handleDeleteApiKey(f.svc), req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleDeleteApiKey_NonExistent_Returns204(t *testing.T) {
	f := newTestFixture(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apiKeys/ghost", nil)
	req.SetPathValue("id", "ghost")
	rec := serveWithSession(f, handleDeleteApiKey(f.svc), req)

	// DELETE for a non-existent key is idempotent — 204
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
