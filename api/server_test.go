package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBuildMux_RegistersAllRoutes(t *testing.T) {
	f := newTestFixture(t)

	// Provide stubs so the handlers don't panic on missing expectations.
	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil).Maybe()
	f.userStore.On("GetUserByUsername", mock.Anything, mock.Anything).
		Return(nil, assert.AnError).Maybe()

	mux := buildMux(0, f.svc)
	wrapped := f.svc.SessionRegistry().WrapWithSessionMiddleware(mux)

	// Verify that each registered route responds (not 404).
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, APPLICATION_ROUTE_BASE_PATH + "/healthcheck"},
		{http.MethodGet, APPLICATION_ROUTE_BASE_PATH + "/fts"},
		{http.MethodPost, APPLICATION_ROUTE_BASE_PATH + "/fts"},
		{http.MethodPost, APPLICATION_ROUTE_BASE_PATH + "/auth/login"},
		{http.MethodGet, APPLICATION_ROUTE_BASE_PATH + "/proxy/routes"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
			// As long as it's not a 404, the route was registered.
			assert.NotEqual(t, http.StatusNotFound, rec.Code, "route %s %s should be registered", tt.method, tt.path)
		})
	}
}

func TestBuildMux_RegistersUIRoutes(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil).Maybe()

	mux := buildMux(0, f.svc)
	wrapped := f.svc.SessionRegistry().WrapWithSessionMiddleware(mux)

	uiPaths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/ui/login"},
		{http.MethodGet, "/ui/setup"},
		{http.MethodGet, "/ui/dashboard"},
		{http.MethodPost, "/ui/logout"},
	}

	for _, tt := range uiPaths {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
			assert.NotEqual(t, http.StatusNotFound, rec.Code)
		})
	}
}

func TestFullRequest_HealthCheck_ThroughMux(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil).Maybe()

	mux := buildMux(0, f.svc)
	wrapped := f.svc.SessionRegistry().WrapWithSessionMiddleware(mux)

	req := httptest.NewRequest(http.MethodGet, APPLICATION_ROUTE_BASE_PATH+"/healthcheck", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestFullRequest_SecureRoute_WithoutAuth_Returns401(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil).Maybe()
	f.svc.proxies = []*proxy.ProxySnapshot{}

	mux := buildMux(0, f.svc)
	wrapped := f.svc.SessionRegistry().WrapWithSessionMiddleware(mux)

	req := httptest.NewRequest(http.MethodGet, APPLICATION_ROUTE_BASE_PATH+"/proxy/routes", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestFullRequest_FtsEndpoint_Forbidden_AfterFts(t *testing.T) {
	f := newTestFixture(t)

	f.sysConfigStore.On("GetSystemConfiguration").
		Return(&domain.SystemConfiguration{ID: 1, IsFirstTimeSetupConcluded: true}, nil).Maybe()

	mux := buildMux(0, f.svc)
	wrapped := f.svc.SessionRegistry().WrapWithSessionMiddleware(mux)

	// POST /fts is only allowed BEFORE FTS. Since FTS is done, it should be forbidden.
	req := newJSONRequest(t, http.MethodPost, APPLICATION_ROUTE_BASE_PATH+"/fts", CompleteFtsRequest{Password: "Whatever1!"})
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
