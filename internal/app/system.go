package app

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/nunoOliveiraqwe/torii/api/session"
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"github.com/nunoOliveiraqwe/torii/proxy/acme"
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
}

type SystemService interface {
	Start() error
	Stop() error
	StartStopAcme() error
	SessionRegistry() *session.Registry
	GetServiceStore() *ServiceStore
	GetConfiguredProxyServers() []*proxy.ProxySnapshot
	GetGlobalMetricsManager() *metrics.ConnectionMetricsManager
	GetSSEBroker() *SSEBroker
	StartProxy(port int) error
	StopProxy(port int) error
	DeleteProxy(port int) error
	AddHttpListener(conf config.HTTPListener) error
	GetSystemHealth() *SystemHealth
	GetRecentErrors(n int) []metrics.ErrorLogEntry
	GetRecentRequests(n int) []metrics.RequestLogEntry
	GetRecentBlockedEntries(n int) []metrics.BlockLogEntry
}

type systemService struct {
	micro                *proxy.Torii
	db                   *sqlite.DB
	sessions             *session.Registry
	serviceStore         *ServiceStore
	acmeStore            store.AcmeStore
	globalMetricsManager *metrics.ConnectionMetricsManager
	sseBroker            *SSEBroker
	startTime            time.Time
}

func NewSystemService(conf config.AppConfig) (SystemService, error) {
	zap.S().Info("Initializing system service")
	mgr := metrics.NewGlobalMetricsHandler(2, context.Background())

	db := sqlite.NewDB("torii.db")
	if err := db.Open(); err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	acmeStore := sqlite.NewAcmeStore(db)
	var acmeMgr *acme.LegoAcmeManager
	acmeConf, err := acmeStore.GetConfiguration()
	if err != nil {
		zap.S().Warnf("Failed to read ACME configuration from DB: %v", err)
	}
	if acmeConf != nil && acmeConf.Enabled {
		acmeMgr, err = acme.NewLegoAcmeManager(acmeConf, acmeStore)
		if err != nil {
			return nil, fmt.Errorf("failed to create ACME manager: %w", err)
		}
	}

	m, err := proxy.NewTorii(conf.NetConfig, mgr, acmeMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to create micro proxy: %w", err)
	}

	sessions := session.NewRegistry(db, conf.Session)
	svc := &systemService{
		micro:                m,
		db:                   db,
		sessions:             sessions,
		acmeStore:            acmeStore,
		globalMetricsManager: mgr,
		sseBroker:            NewSSEBroker(mgr),
		startTime:            time.Now(),
	}
	svc.serviceStore = NewServiceStore(NewDataStore(db), svc.StartStopAcme, svc.GetConfiguredProxyServers)
	return svc, nil
}

func (sm *systemService) GetServiceStore() *ServiceStore {
	return sm.serviceStore
}

func (sm *systemService) GetGlobalMetricsManager() *metrics.ConnectionMetricsManager {
	return sm.globalMetricsManager
}

func (sm *systemService) GetSSEBroker() *SSEBroker {
	return sm.sseBroker
}

func (sm *systemService) SessionRegistry() *session.Registry {
	return sm.sessions
}

func (sm *systemService) GetSystemHealth() *SystemHealth {
	h := &SystemHealth{
		UptimeSeconds: time.Since(sm.startTime).Seconds(),
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

	return h
}

func (sm *systemService) GetRecentErrors(n int) []metrics.ErrorLogEntry {
	return sm.globalMetricsManager.GetErrorLog().Recent(n)
}

func (sm *systemService) GetRecentRequests(n int) []metrics.RequestLogEntry {
	return sm.globalMetricsManager.GetRequestLog().Recent(n)
}

func (sm *systemService) GetRecentBlockedEntries(n int) []metrics.BlockLogEntry {
	return sm.globalMetricsManager.GetBlockedLog().Recent(n)
}

func (sm *systemService) Start() error {
	zap.S().Info("Starting system service")
	if err := sm.micro.StartAll(); err != nil {
		return fmt.Errorf("failed to start micro proxy: %w", err)
	}
	sm.globalMetricsManager.StartCollectingMetrics()
	zap.S().Info("System service started successfully")
	return nil
}

func (sm *systemService) Stop() error {
	zap.S().Info("Stopping system service")
	sm.sseBroker.Stop()
	sm.micro.StopAcme()
	if err := sm.micro.StopAll(); err != nil {
		return fmt.Errorf("failed to stop micro proxy: %w", err)
	}
	sm.globalMetricsManager.StopCollectingMetrics()

	if sm.db != nil {
		zap.S().Info("Closing database")
		if err := sm.db.Close(); err != nil {
			zap.S().Errorf("Failed to close database: %v", err)
		}
	}

	zap.S().Info("System service stopped successfully")
	return nil
}

func (sm *systemService) StartStopAcme() error {
	zap.S().Info("Reloading ACME manager from DB configuration")
	conf, err := sm.acmeStore.GetConfiguration()
	if err != nil {
		return fmt.Errorf("failed to read ACME configuration: %w", err)
	}
	if conf == nil || !conf.Enabled {
		zap.S().Info("ACME disabled or not configured, stopping ACME manager")
		sm.micro.SwapAcmeManager(nil)
		return nil
	}
	mgr, err := acme.NewLegoAcmeManager(conf, sm.acmeStore)
	if err != nil {
		return fmt.Errorf("failed to create ACME manager: %w", err)
	}
	sm.micro.SwapAcmeManager(mgr)
	zap.S().Info("ACME manager reloaded successfully")
	return nil
}

func (sm *systemService) StartProxy(port int) error {
	zap.S().Infof("Starting proxy server on port %d", port)
	err := sm.micro.StartHttpProxy(port)
	if err != nil {
		return fmt.Errorf("failed to start proxy server on port %d: %w", port, err)
	}
	zap.S().Infof("Proxy server started successfully on port %d", port)
	return nil
}

func (sm *systemService) StopProxy(port int) error {
	zap.S().Infof("Stopping proxy server on port %d", port)
	err := sm.micro.StopHttpProxy(port)
	if err != nil {
		return fmt.Errorf("failed to stop proxy server on port %d: %w", port, err)
	}
	zap.S().Infof("Proxy server stopped successfully on port %d", port)
	return nil
}

func (sm *systemService) DeleteProxy(port int) error {
	zap.S().Infof("Deleting proxy server on port %d", port)
	err := sm.micro.DeleteHttpProxy(port)
	if err != nil {
		return fmt.Errorf("failed to stop proxy server on port %d: %w", port, err)
	}
	zap.S().Infof("Proxy server stopped successfully on port %d", port)
	return nil
}

func (sm *systemService) AddHttpListener(conf config.HTTPListener) error {
	zap.S().Infof("Adding HTTP listener on port %d", conf.Port)
	ctx := context.WithValue(context.Background(), ctxkeys.MetricsMgr, sm.globalMetricsManager)
	if err := sm.micro.AddHttpServer(ctx, conf, nil); err != nil {
		return fmt.Errorf("failed to add HTTP listener on port %d: %w", conf.Port, err)
	}
	zap.S().Infof("HTTP listener added successfully on port %d", conf.Port)
	return nil
}

func (sm *systemService) GetConfiguredProxyServers() []*proxy.ProxySnapshot {
	zap.S().Infof("Getting running proxies")
	return sm.micro.GetProxyConfSnapshots()
}
