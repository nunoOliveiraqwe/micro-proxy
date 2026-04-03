package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGetProxies_Success(t *testing.T) {
	f := newTestFixture(t)

	f.svc.proxies = []*proxy.ProxySnapshot{
		{
			Port:            8080,
			Interface:       "0.0.0.0",
			MiddlewareChain: []string{"RequestId", "Metrics"},
			IsStarted:       true,
			IsUsingHTTPS:    false,
			IsUsingACME:     false,
			Metrics:         []*metrics.Metric{{ConnectionName: "test", RequestCount: 42}},
		},
		{
			Port:            8443,
			Interface:       "0.0.0.0",
			MiddlewareChain: []string{},
			IsStarted:       true,
			IsUsingHTTPS:    true,
			IsUsingACME:     true,
		},
	}

	handler := handleGetProxies(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/routes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var proxies []*proxy.ProxySnapshot
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&proxies))
	assert.Len(t, proxies, 2)
	assert.Equal(t, 8080, proxies[0].Port)
	assert.Equal(t, 8443, proxies[1].Port)
	assert.True(t, proxies[1].IsUsingHTTPS)
}

func TestHandleGetProxies_Empty(t *testing.T) {
	f := newTestFixture(t)
	f.svc.proxies = []*proxy.ProxySnapshot{}

	handler := handleGetProxies(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/routes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var proxies []*proxy.ProxySnapshot
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&proxies))
	assert.Len(t, proxies, 0)
}

func TestHandleGetProxies_Nil(t *testing.T) {
	f := newTestFixture(t)
	f.svc.proxies = nil // nil means error in the handler

	handler := handleGetProxies(f.svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy/routes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
