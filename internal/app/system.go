package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nunoOliveiraqwe/torii/api/session"
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/service"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	"go.uber.org/zap"
)

type SystemHealth struct {
	UptimeSeconds    float64 `json:"uptime_seconds"`
	Goroutines       int     `json:"goroutines"`
	CpuUtilPercent   float64 `json:"cpu_util_percent"`
	ProcessRSSBytes  uint64  `json:"process_rss_bytes"`
	ProcessMemPct    float32 `json:"process_mem_percent"`
	SysMemTotalBytes uint64  `json:"sys_mem_total_bytes"`
	SysMemUsedPct    float64 `json:"sys_mem_used_percent"`
	HeapAllocBytes   uint64  `json:"heap_alloc_bytes"`
	GCPauseTotalNs   uint64  `json:"gc_pause_total_ns"`

	ErrorLogCapacity   int `json:"error_log_capacity"`
	RequestLogCapacity int `json:"request_log_capacity"`
	BlockedLogCapacity int `json:"blocked_log_capacity"`
}

type SystemService interface {
	Start() error
	Stop() error
	SessionRegistry() *session.Registry
	GetServiceStore() *service.ServiceStore
	GetConfiguredProxyServers() []*proxy.ProxySnapshot
	GetGlobalMetricsManager() *metrics.ConnectionMetricsManager
	GetCacheInsightManager() *util.CacheInsightManager
	GetSSEBroker() *SSEBroker
	StartProxy(port int) error
	StopProxy(port int) error
	DeleteProxy(port int) error
	AddHttpListener(conf config.HTTPListener) error
	GetProxyConfig(port int) *config.HTTPListener
	EditProxy(port int, conf config.HTTPListener) error
	GetSystemHealth() *SystemHealth
	GetRecentErrors(n int) []metrics.ErrorLogEntry
	GetRecentRequests(n int) []metrics.RequestLogEntry
	GetRecentBlockedEntries(n int) []metrics.BlockLogEntry
	IsHeadless() bool
	PersistConfig() error
}

func collectSystemHealth(startTime time.Time, mgr *metrics.ConnectionMetricsManager) *SystemHealth {
	h := &SystemHealth{
		UptimeSeconds: time.Since(startTime).Seconds(),
		Goroutines:    runtime.NumGoroutine(),
	}

	if cpuPct, err := cpu.Percent(0, false); err != nil {
		zap.S().Errorf("Failed to get CPU usage: %v", err)
		h.CpuUtilPercent = -1
	} else if len(cpuPct) > 0 {
		h.CpuUtilPercent = cpuPct[0]
	}

	if proc, err := process.NewProcess(int32(os.Getpid())); err == nil {
		if memInfo, err := proc.MemoryInfo(); err == nil {
			h.ProcessRSSBytes = memInfo.RSS
		}
		if pct, err := proc.MemoryPercent(); err == nil {
			h.ProcessMemPct = pct
		}
	}

	if vmStat, err := mem.VirtualMemory(); err == nil {
		h.SysMemTotalBytes = vmStat.Total
		h.SysMemUsedPct = vmStat.UsedPercent
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	h.HeapAllocBytes = ms.HeapAlloc
	h.GCPauseTotalNs = ms.PauseTotalNs

	errCap, reqCap, blkCap := mgr.GetLogCapacities()
	h.ErrorLogCapacity = errCap
	h.RequestLogCapacity = reqCap
	h.BlockedLogCapacity = blkCap

	return h
}

type managedService struct {
	micro                *proxy.Torii
	cacheInsightsManager *util.CacheInsightManager
	db                   *sqlite.DB
	sessions             *session.Registry
	serviceStore         *service.ServiceStore
	globalMetricsManager *metrics.ConnectionMetricsManager
	sseBroker            *SSEBroker
	startTime            time.Time
	configPath           string
	appConfig            config.AppConfig
}

func NewSystemService(conf config.AppConfig, configPath string, dataDir string) (SystemService, error) {
	zap.S().Info("Initializing managed system service")
	mgr := metrics.NewGlobalMetricsHandler(2, context.Background())
	cInMgr := util.NewCacheInsightManager()

	dbPath := filepath.Join(dataDir, "torii.db")
	db := sqlite.NewDB(dbPath)
	if err := db.Open(); err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", dbPath, err)
	}
	zap.S().Infof("Database opened at %s", dbPath)

	serviceStore := service.NewServiceStore(service.NewDataStore(db), conf.Acme)

	m, err := proxy.NewTorii(conf.NetConfig, mgr, cInMgr, serviceStore.GetAcmeService())
	if err != nil {
		return nil, fmt.Errorf("failed to create micro proxy: %w", err)
	}

	sessions := session.NewRegistry(db, conf.Session)
	return &managedService{
		micro:                m,
		db:                   db,
		cacheInsightsManager: cInMgr,
		sessions:             sessions,
		globalMetricsManager: mgr,
		sseBroker:            NewSSEBroker(mgr),
		startTime:            time.Now(),
		configPath:           configPath,
		appConfig:            conf,
		serviceStore:         serviceStore,
	}, nil
}

func (s *managedService) IsHeadless() bool {
	return false
}

func (s *managedService) Start() error {
	zap.S().Info("Starting managed system service")
	if err := s.micro.StartAll(); err != nil {
		return fmt.Errorf("failed to start micro proxy: %w", err)
	}
	s.globalMetricsManager.StartCollectingMetrics()
	s.serviceStore.GetAcmeService().Start()
	zap.S().Info("System service started successfully")
	return nil
}

func (s *managedService) Stop() error {
	zap.S().Info("Stopping managed system service")
	s.sseBroker.Stop()
	s.serviceStore.GetAcmeService().Stop()
	if err := s.micro.StopAll(); err != nil {
		return fmt.Errorf("failed to stop micro proxy: %w", err)
	}
	s.globalMetricsManager.StopCollectingMetrics()

	zap.S().Info("Closing database")
	if err := s.db.Close(); err != nil {
		zap.S().Errorf("Failed to close database: %v", err)
	}

	zap.S().Info("System service stopped successfully")
	return nil
}

func (s *managedService) SessionRegistry() *session.Registry {
	return s.sessions
}

func (s *managedService) GetServiceStore() *service.ServiceStore {
	return s.serviceStore
}

func (s *managedService) GetGlobalMetricsManager() *metrics.ConnectionMetricsManager {
	return s.globalMetricsManager
}

func (s *managedService) GetCacheInsightManager() *util.CacheInsightManager {
	return s.cacheInsightsManager
}

func (s *managedService) GetSSEBroker() *SSEBroker {
	return s.sseBroker
}

func (s *managedService) GetConfiguredProxyServers() []*proxy.ProxySnapshot {
	return s.micro.GetProxyConfSnapshots()
}

func (s *managedService) GetProxyConfig(port int) *config.HTTPListener {
	return s.micro.GetProxyConfig(port)
}

func (s *managedService) GetSystemHealth() *SystemHealth {
	return collectSystemHealth(s.startTime, s.globalMetricsManager)
}

func (s *managedService) GetRecentErrors(n int) []metrics.ErrorLogEntry {
	return s.globalMetricsManager.GetErrorLog().Recent(n)
}

func (s *managedService) GetRecentRequests(n int) []metrics.RequestLogEntry {
	return s.globalMetricsManager.GetRequestLog().Recent(n)
}

func (s *managedService) GetRecentBlockedEntries(n int) []metrics.BlockLogEntry {
	return s.globalMetricsManager.GetBlockedLog().Recent(n)
}

func (s *managedService) StartProxy(port int) error {
	zap.S().Infof("Starting proxy server on port %d", port)
	if err := s.micro.StartHttpProxy(port); err != nil {
		return fmt.Errorf("failed to start proxy server on port %d: %w", port, err)
	}
	zap.S().Infof("Proxy server started successfully on port %d", port)
	return nil
}

func (s *managedService) StopProxy(port int) error {
	zap.S().Infof("Stopping proxy server on port %d", port)
	if err := s.micro.StopHttpProxy(port); err != nil {
		return fmt.Errorf("failed to stop proxy server on port %d: %w", port, err)
	}
	zap.S().Infof("Proxy server stopped successfully on port %d", port)
	return nil
}

func (s *managedService) DeleteProxy(port int) error {
	zap.S().Infof("Deleting proxy server on port %d", port)
	if err := s.micro.DeleteHttpProxy(port); err != nil {
		return fmt.Errorf("failed to delete proxy server on port %d: %w", port, err)
	}
	s.globalMetricsManager.RemoveMetricsForServer(fmt.Sprintf("http-%d", port))
	zap.S().Infof("Proxy server deleted successfully on port %d", port)
	return nil
}

func (s *managedService) AddHttpListener(conf config.HTTPListener) error {
	zap.S().Infof("Adding HTTP listener on port %d", conf.Port)
	ctx := context.WithValue(context.Background(), ctxkeys.MetricsMgr, s.globalMetricsManager)
	ctx = context.WithValue(ctx, ctxkeys.CacheInsightMgr, s.cacheInsightsManager)
	if err := s.micro.AddHttpServer(ctx, conf, nil); err != nil {
		return fmt.Errorf("failed to add HTTP listener on port %d: %w", conf.Port, err)
	}
	s.serviceStore.GetAcmeService().NotifyDomainsChanged()
	zap.S().Infof("HTTP listener added successfully on port %d", conf.Port)
	return nil
}

func (s *managedService) EditProxy(port int, conf config.HTTPListener) error {
	zap.S().Infof("Editing proxy on port %d", port)
	ctx := context.WithValue(context.Background(), ctxkeys.MetricsMgr, s.globalMetricsManager)
	ctx = context.WithValue(ctx, ctxkeys.CacheInsightMgr, s.cacheInsightsManager)

	requiresRestart, err := s.micro.DoesConfigRequireServerRestart(port, conf)
	zap.S().Debugf("Config change for port %d requires server restart: %v", port, requiresRestart)
	if err != nil {
		return err
	}

	if requiresRestart {
		zap.S().Infof("Config change for port %d requires server restart, performing full restart", port)
		wasStarted := s.micro.IsStarted(port)

		if wasStarted {
			if err := s.micro.StopHttpProxy(port); err != nil {
				return fmt.Errorf("failed to stop old proxy on port %d: %w", port, err)
			}
		}
		if err := s.micro.DeleteHttpProxy(port); err != nil {
			return fmt.Errorf("failed to delete old proxy on port %d: %w", port, err)
		}
		if err := s.micro.AddHttpServer(ctx, conf, nil); err != nil {
			return fmt.Errorf("failed to recreate proxy on port %d: %w", port, err)
		}
		if wasStarted {
			if err := s.micro.StartHttpProxy(port); err != nil {
				return fmt.Errorf("proxy recreated but failed to start on port %d: %w", port, err)
			}
		}
		zap.S().Infof("Proxy on port %d rebuilt successfully", port)
		return nil
	}

	// Only routes/middleware changed — hot-swap the handler (preserves connections + middleware caches)
	//H2C is a special case
	if err := s.micro.HotSwapHandler(ctx, port, conf, nil); err != nil {
		return fmt.Errorf("failed to edit proxy on port %d: %w", port, err)
	}
	s.serviceStore.GetAcmeService().NotifyDomainsChanged()
	zap.S().Infof("Proxy on port %d edited successfully", port)
	return nil
}

func (s *managedService) PersistConfig() error {
	if s.configPath == "" {
		zap.S().Warn("No config file path set, skipping config persistence")
		return nil
	}
	s.appConfig.NetConfig.HTTPListeners = s.micro.GetAllHTTPConfigs()
	if err := config.SaveConfiguration(s.configPath, s.appConfig); err != nil {
		return fmt.Errorf("failed to persist configuration: %w", err)
	}
	zap.S().Info("Configuration persisted to disk")
	return nil
}
