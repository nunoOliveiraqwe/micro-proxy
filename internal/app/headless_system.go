package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nunoOliveiraqwe/torii/api/session"
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/bus"
	"github.com/nunoOliveiraqwe/torii/internal/service"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"github.com/nunoOliveiraqwe/torii/internal/subsystem"
	"github.com/nunoOliveiraqwe/torii/internal/subsystem/activity"
	cacheSub "github.com/nunoOliveiraqwe/torii/internal/subsystem/cache"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"go.uber.org/zap"
)

type headlessService struct {
	micro                *proxy.Torii
	cacheSubsystem       *cacheSub.Subsystem
	globalMetricsManager *metrics.ConnectionMetricsManager
	startTime            time.Time
	db                   *sqlite.DB
	serviceStore         *service.ServiceStore
	eventBus             bus.Bus
	subManager           *subsystem.Manager
}

func NewHeadlessService(conf config.AppConfig, dataDir string) (SystemService, error) {
	zap.S().Info("Initializing headless service (proxy only)")
	mgr := metrics.NewGlobalMetricsHandler(2, context.Background())
	cacheSubsystem := cacheSub.NewSubsystem()

	var db *sqlite.DB
	var svcStore *service.ServiceStore
	var acmeSvc *service.AcmeService

	if anyAcmeEnabled(conf.Acme) {
		dbPath := filepath.Join(dataDir, "torii.db")
		db = sqlite.NewDB(dbPath)
		if err := db.Open(); err != nil {
			return nil, fmt.Errorf("failed to open database at %s (needed for ACME): %w", dbPath, err)
		}
		zap.S().Infof("Headless: database opened at %s (ACME enabled)", dbPath)
		svcStore = service.NewServiceStore(service.NewDataStore(db), conf.Acme)
		acmeSvc = svcStore.GetAcmeService()
	}
	eventBus := bus.NewEventBus()

	m, err := proxy.NewTorii(conf.NetConfig, mgr, cacheSubsystem, acmeSvc, eventBus)
	if err != nil {
		return nil, fmt.Errorf("failed to create micro proxy: %w", err)
	}
	subManager := subsystem.NewSubsystemManager(eventBus, cacheSubsystem)

	return &headlessService{
		eventBus:             eventBus,
		subManager:           subManager,
		micro:                m,
		cacheSubsystem:       cacheSubsystem,
		globalMetricsManager: mgr,
		startTime:            time.Now(),
		db:                   db,
		serviceStore:         svcStore,
	}, nil
}

func (s *headlessService) IsHeadless() bool { return true }

func (s *headlessService) Start() error {
	zap.S().Info("Starting headless proxy")
	s.eventBus.Start()
	if err := s.subManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize subsystems: %w", err)
	}
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
	if err := s.subManager.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown subsystems: %w", err)
	}
	s.globalMetricsManager.StopCollectingMetrics()
	s.eventBus.Stop()
	if s.db != nil {
		zap.S().Info("Closing database")
		if err := s.db.Close(); err != nil {
			zap.S().Errorf("Failed to close database: %v", err)
		}
	}
	zap.S().Info("Headless proxy stopped successfully")
	return nil
}

func (s *headlessService) GetSessionRegistry() *session.UserRegistry {
	return nil
}

func (s *headlessService) GetEventBus() bus.Bus {
	return s.eventBus
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

func (s *headlessService) GetCacheSubsystem() *cacheSub.Subsystem {
	return s.cacheSubsystem
}

func (s *headlessService) GetConfiguredProxyServers() []*proxy.ProxySnapshot {
	return s.micro.GetProxyConfSnapshots()
}

func (s *headlessService) GetProxyConfig(port int) *config.HTTPListener {
	return s.micro.GetProxyConfig(port)
}

func (s *headlessService) GetToriiVersion() string {
	return ""
}

func (s *headlessService) GetSystemHealth() *SystemHealth {
	return collectSystemHealth(s.startTime, s.subManager.GetActivitySubsystem())
}

func (s *headlessService) GetSubsystemManager() *subsystem.Manager {
	return s.subManager
}

func (s *headlessService) GetRecentErrors(n int) []activity.ErrorLogEntry {
	return s.subManager.GetActivitySubsystem().ErrorLog.Recent(n)
}

func (s *headlessService) GetRecentRequests(n int) []activity.RequestLogEntry {
	return s.subManager.GetActivitySubsystem().RequestLog.Recent(n)
}

func (s *headlessService) GetRecentBlockedEntries(n int) []activity.BlockLogEntry {
	return s.subManager.GetActivitySubsystem().BlockLog.Recent(n)
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

func anyAcmeEnabled(confs []*config.AcmeConfig) bool {
	for _, c := range confs {
		if c != nil && c.Enabled {
			return true
		}
	}
	return false
}
