package torii

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/api"
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/logging"
	"go.uber.org/zap"
)

var Version = "0.1.0"
var Build = "non commited"
var BuildTime = "unknown"

type Application struct {
	appConfig config.AppConfig
	flags     *Flags
	service   app.SystemService
	apiServer *http.Server
	debug     *debugServers
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
	conf, err := config.LoadConfiguration(a.flags.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration from %q: %w", a.flags.ConfigPath, err)
	}
	a.appConfig = conf
	a.applyFlagOverrides()
	return nil
}

func (a *Application) applyFlagOverrides() {
	if a.flags.LogLevel != nil && *a.flags.LogLevel != "" {
		a.appConfig.LogConfig.LogLevel = *a.flags.LogLevel
	}
}

func (a *Application) InitLogger() {
	logging.InitLogger(a.appConfig.LogConfig)
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
	readOnly := a.flags.ReadOnly != nil && *a.flags.ReadOnly
	svc, err := app.NewSystemService(a.appConfig, a.flags.ConfigPath, readOnly)
	if err != nil {
		return fmt.Errorf("failed to create system service: %w", err)
	}
	a.service = svc

	a.apiServer = api.StartServer(a.appConfig.APIServer, a.service)

	ln, err := net.Listen("tcp", a.apiServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to bind API server on %s: %w", a.apiServer.Addr, err)
	}
	zap.S().Infof("API server listening on %s", a.apiServer.Addr)

	go func() {
		if err := a.apiServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zap.S().Fatalf("API server failed: %v", err)
		}
	}()

	if err := a.service.Start(); err != nil {
		return fmt.Errorf("failed to start system service: %w", err)
	}

	a.RunDebugMode()
	zap.S().Info("Application started successfully")
	return nil
}

func (a *Application) Shutdown(ctx context.Context) error {
	zap.S().Info("Shutting down application...")

	a.ShutdownDebug(ctx)

	if a.service != nil {
		zap.S().Info("Closing SSE broker to release streaming connections")
		a.service.GetSSEBroker().Stop()
	}

	if a.apiServer != nil {
		zap.S().Info("Shutting down API server")
		if err := a.apiServer.Shutdown(ctx); err != nil {
			zap.S().Errorf("API server shutdown error: %v", err)
		}
	}

	if a.service != nil {
		zap.S().Info("Stopping system service")
		if err := a.service.Stop(); err != nil {
			return fmt.Errorf("failed to stop system service: %w", err)
		}
	}

	zap.S().Info("Application shut down gracefully")
	return nil
}
