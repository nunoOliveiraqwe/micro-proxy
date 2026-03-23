package app

import (
	"fmt"

	"github.com/nunoOliveiraqwe/micro-proxy/api/session"
	"github.com/nunoOliveiraqwe/micro-proxy/config"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/sqlite"
	"github.com/nunoOliveiraqwe/micro-proxy/proxy"
	"go.uber.org/zap"
)

type SystemService interface {
	Start() error
	Stop() error
	SessionRegistry() *session.Registry
	GetServiceStore() *ServiceStore
	GetConfiguredProxyServers() []*proxy.ProxyConfigSnapshot
}

type systemService struct {
	micro        *proxy.MicroProxy
	db           *sqlite.DB
	sessions     *session.Registry
	serviceStore *ServiceStore
}

func NewSystemService(conf config.AppConfig) (SystemService, error) {
	zap.S().Info("Initializing system service")

	m, err := proxy.NewMicroProxy(conf.NetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create micro proxy: %w", err)
	}

	db := sqlite.NewDB("micro-proxy.db")
	if err := db.Open(); err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	sessions := session.NewRegistry(db, conf.Session)
	return &systemService{
		micro:        m,
		db:           db,
		sessions:     sessions,
		serviceStore: NewServiceStore(NewDataStore(db)),
	}, nil
}

func (sm *systemService) GetServiceStore() *ServiceStore {
	return sm.serviceStore
}

func (sm *systemService) SessionRegistry() *session.Registry {
	return sm.sessions
}

func (sm *systemService) Start() error {
	zap.S().Info("Starting system service")
	if err := sm.micro.Start(); err != nil {
		return fmt.Errorf("failed to start micro proxy: %w", err)
	}
	zap.S().Info("System service started successfully")
	return nil
}

func (sm *systemService) Stop() error {
	zap.S().Info("Stopping system service")
	if err := sm.micro.Stop(); err != nil {
		return fmt.Errorf("failed to stop micro proxy: %w", err)
	}
	zap.S().Info("System service stopped successfully")
	return nil
}

func (sm *systemService) GetConfiguredProxyServers() []*proxy.ProxyConfigSnapshot {
	zap.S().Infof("Getting running proxies")
	return sm.micro.GetProxyConfSnapshots()
}
