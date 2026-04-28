package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nunoOliveiraqwe/torii/api/session"
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/service"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"go.uber.org/zap"
)

type headlessService struct {
	micro                *proxy.Torii
	cacheInsightsManager *util.CacheInsightManager
	globalMetricsManager *metrics.ConnectionMetricsManager
	startTime            time.Time
	db                   *sqlite.DB
	serviceStore         *service.ServiceStore
}

func NewHeadlessService(conf config.AppConfig, dataDir string) (SystemService, error) {
	zap.S().Info("Initializing headless service (proxy only)")
	mgr := metrics.NewGlobalMetricsHandler(2, context.Background())
	cInMgr := util.NewCacheInsightManager()

	var db *sqlite.DB
	var svcStore *service.ServiceStore
	var acmeSvc *service.AcmeService

	if conf.Acme != nil && conf.Acme.Enabled {
		dbPath := filepath.Join(dataDir, "torii.db")
		db = sqlite.NewDB(dbPath)
		if err := db.Open(); err != nil {
			return nil, fmt.Errorf("failed to open database at %s (needed for ACME): %w", dbPath, err)
		}
		zap.S().Infof("Headless: database opened at %s (ACME enabled)", dbPath)
		svcStore = service.NewServiceStore(service.NewDataStore(db), conf.Acme)
		acmeSvc = svcStore.GetAcmeService()
	}

	m, err := proxy.NewTorii(conf.NetConfig, mgr, cInMgr, acmeSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to create micro proxy: %w", err)
	}

	return &headlessService{
		micro:                m,
		cacheInsightsManager: cInMgr,
		globalMetricsManager: mgr,
		startTime:            time.Now(),
		db:                   db,
		serviceStore:         svcStore,
	}, nil
}

func (s *headlessService) IsHeadless() bool { return true }

func (s *headlessService) Start() error {
	zap.S().Info("Starting headless proxy")
	if err := s.micro.StartAll(); err != nil {
		return fmt.Errorf("failed to start micro proxy: %w", err)
	}
	s.globalMetricsManager.StartCollectingMetrics()
	if s.serviceStore != nil {
		s.serviceStore.GetAcmeService().Start()
	}
	zap.S().Info("Headless proxy started successfully")
	return nil
}

func (s *headlessService) Stop() error {
	zap.S().Info("Stopping headless proxy")
	if s.serviceStore != nil {
		s.serviceStore.GetAcmeService().Stop()
	}
	if err := s.micro.StopAll(); err != nil {
		return fmt.Errorf("failed to stop micro proxy: %w", err)
	}
	s.globalMetricsManager.StopCollectingMetrics()
	if s.db != nil {
		zap.S().Info("Closing database")
		if err := s.db.Close(); err != nil {
			zap.S().Errorf("Failed to close database: %v", err)
		}
	}
	zap.S().Info("Headless proxy stopped successfully")
	return nil
}

func (s *headlessService) SessionRegistry() *session.Registry {
	return nil
}

func (s *headlessService) GetServiceStore() *service.ServiceStore {
	return s.serviceStore
}

func (s *headlessService) GetSSEBroker() *SSEBroker {
	return nil
}

func (s *headlessService) GetGlobalMetricsManager() *metrics.ConnectionMetricsManager {
	return s.globalMetricsManager
}

func (s *headlessService) GetCacheInsightManager() *util.CacheInsightManager {
	return s.cacheInsightsManager
}

func (s *headlessService) GetConfiguredProxyServers() []*proxy.ProxySnapshot {
	return s.micro.GetProxyConfSnapshots()
}

func (s *headlessService) GetProxyConfig(port int) *config.HTTPListener {
	return s.micro.GetProxyConfig(port)
}

func (s *headlessService) GetSystemHealth() *SystemHealth {
	return collectSystemHealth(s.startTime, s.globalMetricsManager)
}

func (s *headlessService) GetRecentErrors(n int) []metrics.ErrorLogEntry {
	return s.globalMetricsManager.GetErrorLog().Recent(n)
}

func (s *headlessService) GetRecentRequests(n int) []metrics.RequestLogEntry {
	return s.globalMetricsManager.GetRequestLog().Recent(n)
}

func (s *headlessService) GetRecentBlockedEntries(n int) []metrics.BlockLogEntry {
	return s.globalMetricsManager.GetBlockedLog().Recent(n)
}

func (s *headlessService) PersistConfig() error { return nil }

func (s *headlessService) StartProxy(port int) error {
	return fmt.Errorf("cannot mutate proxies in headless mode")
}

func (s *headlessService) StopProxy(port int) error {
	return fmt.Errorf("cannot mutate proxies in headless mode")
}

func (s *headlessService) DeleteProxy(port int) error {
	return fmt.Errorf("cannot mutate proxies in headless mode")
}

func (s *headlessService) AddHttpListener(conf config.HTTPListener) error {
	return fmt.Errorf("cannot mutate proxies in headless mode")
}

func (s *headlessService) EditProxy(port int, conf config.HTTPListener) error {
	return fmt.Errorf("cannot mutate proxies in headless mode")
}
