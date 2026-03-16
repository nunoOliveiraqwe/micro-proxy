package microProxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nunoOliveiraqwe/micro-proxy/configuration"
	"github.com/nunoOliveiraqwe/micro-proxy/internal/manager"
	"github.com/nunoOliveiraqwe/micro-proxy/log"
	"github.com/nunoOliveiraqwe/micro-proxy/server"
	"go.uber.org/zap"
)

var Version = "0.0.x"
var Build = "non commited"
var BuildTime = "unknown"

type Application struct {
	appConfig configuration.ApplicationConfiguration
	flags     *Flags
	manager   manager.SystemManager
	apiServer *http.Server
}

func NewApplication() *Application {
	return &Application{
		flags: RegisterFlags(),
	}
}

func (a *Application) ParseFlags() {
	a.flags.ParseFlags()
}

func (a *Application) LoadConfiguration() error {
	conf, err := configuration.LoadConfiguration(a.flags.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration from %q: %w", a.flags.ConfigPath, err)
	}
	a.appConfig = conf
	a.applyFlagOverrides()
	zap.S().Infof("Configuration loaded from %s", a.flags.ConfigPath)
	return nil
}

func (a *Application) applyFlagOverrides() {
	if a.flags.Debug != nil && *a.flags.Debug {
		a.appConfig.LogConfig.Debug = true
	}
	if a.flags.LogLevel != nil && *a.flags.LogLevel != "" {
		a.appConfig.LogConfig.LogLevel = *a.flags.LogLevel
	}
}

func (a *Application) InitLogger() {
	log.InitLogger(a.appConfig.LogConfig)
}

func (a *Application) Validate() error {
	if a.flags == nil {
		return fmt.Errorf("flags not initialized")
	}
	if a.appConfig.APIServer.Port <= 0 || a.appConfig.APIServer.Port > 65535 {
		return fmt.Errorf("invalid API server port: %d", a.appConfig.APIServer.Port)
	}
	return nil
}

func (a *Application) Start() error {
	mgr, err := manager.NewSystemManager(a.appConfig.NetConfig)
	if err != nil {
		return fmt.Errorf("failed to create system manager: %w", err)
	}
	a.manager = mgr

	if err := a.manager.Start(); err != nil {
		return fmt.Errorf("failed to start system manager: %w", err)
	}

	a.apiServer = server.StartServer(a.appConfig.APIServer, a.manager)
	go func() {
		zap.S().Infof("Starting API server on %s", a.apiServer.Addr)
		if err := a.apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zap.S().Fatalf("API server failed: %v", err)
		}
	}()

	zap.S().Info("Application started successfully")
	return nil
}

func (a *Application) Shutdown(ctx context.Context) error {
	zap.S().Info("Shutting down application...")

	if a.apiServer != nil {
		zap.S().Info("Shutting down API server")
		if err := a.apiServer.Shutdown(ctx); err != nil {
			zap.S().Errorf("API server shutdown error: %v", err)
		}
	}

	if a.manager != nil {
		zap.S().Info("Stopping system manager")
		if err := a.manager.Stop(); err != nil {
			return fmt.Errorf("failed to stop system manager: %w", err)
		}
	}

	zap.S().Info("Application shut down gracefully")
	return nil
}
