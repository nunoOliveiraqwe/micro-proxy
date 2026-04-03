package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nunoOliveiraqwe/torii/api/session"
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func init() {
	// Ensure a no-op logger is available so handler code that calls zap.S()
	// does not panic during tests.
	logger := zap.NewNop()
	zap.ReplaceGlobals(logger)
}

// ---------------------------------------------------------------------------
// Mock store implementations
// ---------------------------------------------------------------------------

type mockUserStore struct {
	mock.Mock
}

func (m *mockUserStore) GetUserById(ctx context.Context, id int) (*domain.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockUserStore) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockUserStore) GetRolesForUser(ctx context.Context, username string) ([]domain.Role, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Role), args.Error(1)
}

func (m *mockUserStore) UpdateUser(ctx context.Context, user *domain.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *mockUserStore) InsertUser(ctx context.Context, user *domain.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

// ---

type mockSysConfigStore struct {
	mock.Mock
}

func (m *mockSysConfigStore) GetSystemConfiguration() (*domain.SystemConfiguration, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SystemConfiguration), args.Error(1)
}

func (m *mockSysConfigStore) UpdateSystemConfiguration(cfg *domain.SystemConfiguration) error {
	args := m.Called(cfg)
	return args.Error(0)
}

// ---

type mockRoleStore struct {
	mock.Mock
}

func (m *mockRoleStore) GetRoleById(ctx context.Context, id int) (*domain.Role, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Role), args.Error(1)
}

func (m *mockRoleStore) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Role), args.Error(1)
}

// ---

type mockProxyMetricsStore struct {
	mock.Mock
}

func (m *mockProxyMetricsStore) GetGlobalProxyMetrics(ctx context.Context) (*domain.GlobalProxyMetrics, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.GlobalProxyMetrics), args.Error(1)
}

func (m *mockProxyMetricsStore) UpdateGlobalProxyMetrics(ctx context.Context, metrics *domain.GlobalProxyMetrics) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

// ---------------------------------------------------------------------------
// Mock SystemService
// ---------------------------------------------------------------------------

type testSystemService struct {
	serviceStore   *app.ServiceStore
	sessions       *session.Registry
	proxies        []*proxy.ProxySnapshot
	metricsManager *metrics.ConnectionMetricsManager
}

func (t *testSystemService) Start() error                                      { return nil }
func (t *testSystemService) Stop() error                                       { return nil }
func (t *testSystemService) ReloadAcme() error                                 { return nil }
func (t *testSystemService) SessionRegistry() *session.Registry                { return t.sessions }
func (t *testSystemService) GetServiceStore() *app.ServiceStore                { return t.serviceStore }
func (t *testSystemService) GetConfiguredProxyServers() []*proxy.ProxySnapshot { return t.proxies }
func (t *testSystemService) GetGlobalMetricsManager() *metrics.ConnectionMetricsManager {
	return t.metricsManager
}
func (t *testSystemService) GetSSEBroker() *app.SSEBroker { return nil }
func (t *testSystemService) StartProxy(port int) error    { return nil }
func (t *testSystemService) StopProxy(port int) error     { return nil }
func (t *testSystemService) GetSystemHealth() *app.SystemHealth {
	return &app.SystemHealth{}
}
func (t *testSystemService) GetRecentErrors(n int) []metrics.ErrorEntry {
	return []metrics.ErrorEntry{}
}
func (t *testSystemService) GetRecentRequests(n int) []metrics.RequestLogEntry {
	return []metrics.RequestLogEntry{}
}

// ---------------------------------------------------------------------------
// Test fixture builder
// ---------------------------------------------------------------------------

type testFixture struct {
	svc            *testSystemService
	userStore      *mockUserStore
	sysConfigStore *mockSysConfigStore
	roleStore      *mockRoleStore
	metricsStore   *mockProxyMetricsStore
	db             *sqlite.DB
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()

	db := sqlite.NewDB(":memory:")
	if err := db.Open(); err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	reg := session.NewRegistry(db, config.SessionConfig{
		Lifetime:        24 * time.Hour,
		IdleTimeout:     1 * time.Hour,
		CleanupInterval: 0,
		CookieHttpOnly:  true,
	})

	mus := new(mockUserStore)
	mscs := new(mockSysConfigStore)
	mrs := new(mockRoleStore)
	mpms := new(mockProxyMetricsStore)

	dataStore := &app.DataStore{
		UserStore:         store.UserStore(mus),
		RoleStore:         store.RoleStore(mrs),
		SystemConfigStore: store.SystemConfigStore(mscs),
		ProxyMetricsStore: store.ProxyMetricsStore(mpms),
		AcmeStore:         sqlite.NewAcmeStore(db),
	}
	serviceStore := app.NewServiceStore(dataStore)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mgr := metrics.NewGlobalMetricsHandler(1, ctx)
	mgr.StartCollectingMetrics()
	t.Cleanup(func() { mgr.StopCollectingMetrics() })

	svc := &testSystemService{
		serviceStore:   serviceStore,
		sessions:       reg,
		metricsManager: mgr,
	}

	return &testFixture{
		svc:            svc,
		userStore:      mus,
		sysConfigStore: mscs,
		roleStore:      mrs,
		metricsStore:   mpms,
		db:             db,
	}
}

// ---------------------------------------------------------------------------
// HTTP test helpers
// ---------------------------------------------------------------------------

// newJSONRequest creates an HTTP request with the given method, path, and JSON body.
func newJSONRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("failed to encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// serveWithSession wraps a handler with the session middleware and serves
// the request, returning the recorded response.
func serveWithSession(f *testFixture, handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	wrapped := f.svc.SessionRegistry().WrapWithSessionMiddleware(handler)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	return rec
}
