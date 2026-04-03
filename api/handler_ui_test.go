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
// handleRoot
// ---------------------------------------------------------------------------

func TestUIHandleRoot_RedirectsToSetup_WhenFtsNotDone(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: false}, nil)

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := serveWithSession(f, http.HandlerFunc(h.handleRoot), req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/setup", rec.Header().Get("Location"))
}

func TestUIHandleRoot_RedirectsToLogin_WhenFtsDoneNoSession(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil)

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := serveWithSession(f, http.HandlerFunc(h.handleRoot), req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/login", rec.Header().Get("Location"))
}

func TestUIHandleRoot_RedirectsToDashboard_WhenAuthenticated(t *testing.T) {
	f := newTestFixture(t)
	hashed := hashPassword(t, "Secret1!")

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil)
	f.userStore.On("GetUserByUsername", mock.Anything, "admin").
		Return(&domain.User{ID: 1, Username: "admin", Password: hashed}, nil)

	h := newUIHandler(f.svc)

	// First, create a session via login.
	loginBody := LoginRequest{Username: "admin", Password: "Secret1!"}
	loginReq := newJSONRequest(t, http.MethodPost, "/api/v1/auth/login", loginBody)
	loginRec := serveWithSession(f, handleLogin(f.svc), loginReq)
	assert.Equal(t, http.StatusOK, loginRec.Code)

	// Grab the session cookie.
	cookies := loginRec.Result().Cookies()

	// Now request root with the session cookie.
	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		rootReq.AddCookie(c)
	}
	rec := serveWithSession(f, http.HandlerFunc(h.handleRoot), rootReq)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/dashboard", rec.Header().Get("Location"))
}

// ---------------------------------------------------------------------------
// handleLoginPage
// ---------------------------------------------------------------------------

func TestUIHandleLoginPage_RedirectsToSetup_WhenFtsNotDone(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: false}, nil)

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/ui/login", nil)
	rec := serveWithSession(f, http.HandlerFunc(h.handleLoginPage), req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/setup", rec.Header().Get("Location"))
}

func TestUIHandleLoginPage_RendersLogin_WhenFtsDoneNoSession(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil)

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/ui/login", nil)
	rec := serveWithSession(f, http.HandlerFunc(h.handleLoginPage), req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

// ---------------------------------------------------------------------------
// handleSetupPage
// ---------------------------------------------------------------------------

func TestUIHandleSetupPage_RendersSetup_WhenFtsNotDone(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: false}, nil)

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/ui/setup", nil)
	rec := serveWithSession(f, http.HandlerFunc(h.handleSetupPage), req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

func TestUIHandleSetupPage_RedirectsToLogin_WhenFtsDone(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil)

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/ui/setup", nil)
	rec := serveWithSession(f, http.HandlerFunc(h.handleSetupPage), req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/login", rec.Header().Get("Location"))
}

// ---------------------------------------------------------------------------
// handleDashboardPage
// ---------------------------------------------------------------------------

func TestUIHandleDashboardPage_RedirectsToLogin_WhenNoSession(t *testing.T) {
	f := newTestFixture(t)

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/ui/dashboard", nil)
	rec := serveWithSession(f, http.HandlerFunc(h.handleDashboardPage), req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/login", rec.Header().Get("Location"))
}

func TestUIHandleDashboardPage_RendersDashboard_WhenAuthenticated(t *testing.T) {
	f := newTestFixture(t)
	hashed := hashPassword(t, "Secret1!")

	f.userStore.On("GetUserByUsername", mock.Anything, "admin").
		Return(&domain.User{ID: 1, Username: "admin", Password: hashed}, nil)

	// Create a session first.
	loginBody := LoginRequest{Username: "admin", Password: "Secret1!"}
	loginReq := newJSONRequest(t, http.MethodPost, "/api/v1/auth/login", loginBody)
	loginRec := serveWithSession(f, handleLogin(f.svc), loginReq)
	cookies := loginRec.Result().Cookies()

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/ui/dashboard", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := serveWithSession(f, http.HandlerFunc(h.handleDashboardPage), req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

// ---------------------------------------------------------------------------
// handleLogout
// ---------------------------------------------------------------------------

func TestUIHandleLogout_SetsRedirectHeader(t *testing.T) {
	f := newTestFixture(t)

	h := newUIHandler(f.svc)
	req := httptest.NewRequest(http.MethodPost, "/ui/logout", nil)
	rec := serveWithSession(f, http.HandlerFunc(h.handleLogout), req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/ui/login", rec.Header().Get("HX-Redirect"))
}
