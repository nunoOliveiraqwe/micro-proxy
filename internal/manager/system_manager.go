package manager

import (
	"fmt"

	"github.com/nunoOliveiraqwe/micro-proxy/configuration"
	"github.com/nunoOliveiraqwe/micro-proxy/proxy"
	"go.uber.org/zap"
)

type SystemManager interface {
	Start() error
	Stop() error
	GetProxy() *proxy.MicroProxy
}

type systemManager struct {
	micro *proxy.MicroProxy
}

func NewSystemManager(conf configuration.NetworkConfiguration) (SystemManager, error) {
	zap.S().Info("Initializing system manager")
	m, err := proxy.NewMicroProxy(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create micro proxy: %w", err)
	}

	return &systemManager{
		micro: m,
	}, nil
}

func (sm *systemManager) Start() error {
	zap.S().Info("Starting system manager")
	if err := sm.micro.Start(); err != nil {
		return fmt.Errorf("failed to start micro proxy: %w", err)
	}
	zap.S().Info("System manager started successfully")
	return nil
}

func (sm *systemManager) Stop() error {
	zap.S().Info("Stopping system manager")
	if err := sm.micro.Stop(); err != nil {
		return fmt.Errorf("failed to stop micro proxy: %w", err)
	}
	zap.S().Info("System manager stopped successfully")
	return nil
}

func (sm *systemManager) GetProxy() *proxy.MicroProxy {
	return sm.micro
}
