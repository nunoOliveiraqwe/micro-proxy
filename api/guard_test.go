package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ---------------------------------------------------------------------------
// checkIfRouteIsAllowedIfFtsIsNotDone
// ---------------------------------------------------------------------------

func TestFtsGuard_AllowedBeforeAndAfter(t *testing.T) {
	f := newTestFixture(t)
	// When both flags are true, the guard always passes — no store call needed.
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	guarded := checkIfRouteIsAllowedIfFtsIsNotDone(next, true, true, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestFtsGuard_AllowedOnlyAfterFts_FtsDone(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	guarded := checkIfRouteIsAllowedIfFtsIsNotDone(next, false, true, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	assert.True(t, called)
}

func TestFtsGuard_AllowedOnlyAfterFts_FtsNotDone(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: false}, nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	guarded := checkIfRouteIsAllowedIfFtsIsNotDone(next, false, true, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestFtsGuard_AllowedOnlyBeforeFts_FtsNotDone(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: false}, nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	guarded := checkIfRouteIsAllowedIfFtsIsNotDone(next, true, false, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	assert.True(t, called)
}

func TestFtsGuard_AllowedOnlyBeforeFts_FtsDone(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	guarded := checkIfRouteIsAllowedIfFtsIsNotDone(next, true, false, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestFtsGuard_NeitherAllowed(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: false}, nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	guarded := checkIfRouteIsAllowedIfFtsIsNotDone(next, false, false, f.svc)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// ---------------------------------------------------------------------------
// isAuthenticatedRequest
// ---------------------------------------------------------------------------

func TestAuthGuard_NoSession(t *testing.T) {
	f := newTestFixture(t)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	guarded := isAuthenticatedRequest(next, f.svc)
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	rec := serveWithSession(f, guarded, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthGuard_WithValidSession(t *testing.T) {
	f := newTestFixture(t)
	hashed := hashPassword(t, "Secret1!")

	f.userStore.On("GetUserByUsername", mock.Anything, "admin").
		Return(&domain.User{ID: 1, Username: "admin", Password: hashed}, nil)

	// Create session via login
	loginBody := LoginRequest{Username: "admin", Password: "Secret1!"}
	loginReq := newJSONRequest(t, http.MethodPost, "/api/v1/auth/login", loginBody)
	loginRec := serveWithSession(f, handleLogin(f.svc), loginReq)
	cookies := loginRec.Result().Cookies()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	guarded := isAuthenticatedRequest(next, f.svc)
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := serveWithSession(f, guarded, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}
