package app

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/api/session"
	"github.com/nunoOliveiraqwe/micro-proxy/config"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/sqlite"
	"github.com/nunoOliveiraqwe/micro-proxy/metrics"
	"github.com/nunoOliveiraqwe/micro-proxy/proxy"
	"go.uber.org/zap"
)

type SystemHealth struct {
	UptimeSeconds  float64 `json:"uptime_seconds"`
	Goroutines     int     `json:"goroutines"`
	MemAllocBytes  uint64  `json:"mem_alloc_bytes"`
	MemSysBytes    uint64  `json:"mem_sys_bytes"`
	HeapAllocBytes uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes   uint64  `json:"heap_sys_bytes"`
	GCPauseTotalNs uint64  `json:"gc_pause_total_ns"`
	NumGC          uint32  `json:"num_gc"`
}

type SystemService interface {
	Start() error
	Stop() error
	SessionRegistry() *session.Registry
	GetServiceStore() *ServiceStore
	GetConfiguredProxyServers() []*proxy.ProxySnapshot
	GetGlobalMetricsManager() *metrics.ConnectionMetricsManager
	GetSSEBroker() *SSEBroker
	StartProxy(port int) error
	StopProxy(port int) error
	GetSystemHealth() *SystemHealth
	GetRecentErrors(n int) []metrics.ErrorEntry
	GetRecentRequests(n int) []metrics.RequestLogEntry
}

type systemService struct {
	micro                *proxy.MicroProxy
	db                   *sqlite.DB
	sessions             *session.Registry
	serviceStore         *ServiceStore
	globalMetricsManager *metrics.ConnectionMetricsManager
	sseBroker            *SSEBroker
	startTime            time.Time
}

func NewSystemService(conf config.AppConfig) (SystemService, error) {
	zap.S().Info("Initializing system service")
	mgr := metrics.NewGlobalMetricsHandler(2, context.Background())
	m, err := proxy.NewMicroProxy(conf.NetConfig, mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to create micro proxy: %w", err)
	}

	db := sqlite.NewDB("micro-proxy.db")
	if err := db.Open(); err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	sessions := session.NewRegistry(db, conf.Session)
	return &systemService{
		micro:                m,
		db:                   db,
		sessions:             sessions,
		serviceStore:         NewServiceStore(NewDataStore(db)),
		globalMetricsManager: mgr,
		sseBroker:            NewSSEBroker(mgr),
		startTime:            time.Now(),
	}, nil
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
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return &SystemHealth{
		UptimeSeconds:  time.Since(sm.startTime).Seconds(),
		Goroutines:     runtime.NumGoroutine(),
		MemAllocBytes:  mem.Alloc,
		MemSysBytes:    mem.Sys,
		HeapAllocBytes: mem.HeapAlloc,
		HeapSysBytes:   mem.HeapSys,
		GCPauseTotalNs: mem.PauseTotalNs,
		NumGC:          mem.NumGC,
	}
}

func (sm *systemService) GetRecentErrors(n int) []metrics.ErrorEntry {
	return sm.globalMetricsManager.GetErrorLog().Recent(n)
}

func (sm *systemService) GetRecentRequests(n int) []metrics.RequestLogEntry {
	return sm.globalMetricsManager.GetRequestLog().Recent(n)
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
	if err := sm.micro.StopAll(); err != nil {
		return fmt.Errorf("failed to stop micro proxy: %w", err)
	}
	sm.globalMetricsManager.StopCollectingMetrics()
	zap.S().Info("System service stopped successfully")
	return nil
}

func (sm *systemService) StartProxy(port int) error {
	zap.S().Infof("Starting proxy server on port %d", port)
	err := sm.micro.StartProxy(port)
	if err != nil {
		return fmt.Errorf("failed to start proxy server on port %d: %w", port, err)
	}
	zap.S().Infof("Proxy server started successfully on port %d", port)
	return nil
}

func (sm *systemService) StopProxy(port int) error {
	zap.S().Infof("Stopping proxy server on port %d", port)
	err := sm.micro.StopProxy(port)
	if err != nil {
		return fmt.Errorf("failed to stop proxy server on port %d: %w", port, err)
	}
	zap.S().Infof("Proxy server stopped successfully on port %d", port)
	return nil
}

func (sm *systemService) GetConfiguredProxyServers() []*proxy.ProxySnapshot {
	zap.S().Infof("Getting running proxies")
	return sm.micro.GetProxyConfSnapshots()
}
