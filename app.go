package torii

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/nunoOliveiraqwe/torii/api"
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/fsutil"
	"github.com/nunoOliveiraqwe/torii/logging"
	"go.uber.org/zap"
)

var Version = "0.1.0"
var Build = "non commited"
var BuildTime = "unknown"

const workingConfigName = "torii-conf.yaml"

type Application struct {
	appConfig         config.AppConfig
	flags             *Flags
	service           app.SystemService
	apiServer         *http.Server
	debug             *debugServers
	dataDir           string
	workingConfigPath string
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
	if a.flags.IsHeadless() {
		return a.loadHeadless()
	}
	return a.loadManaged()
}

func logBoot(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[boot] "+format+"\n", args...)
}

func (a *Application) loadHeadless() error {
	if a.flags.ConfigPath == "" {
		return fmt.Errorf("--config is required in headless mode")
	}

	// Resolve data-dir even in headless — ACME needs it for cert persistence.
	absDir, err := filepath.Abs(a.flags.DataDir)
	if err != nil {
		return fmt.Errorf("failed to resolve data-dir %q: %w", a.flags.DataDir, err)
	}
	a.dataDir = absDir
	if err := os.MkdirAll(a.dataDir, 0750); err != nil {
		return fmt.Errorf("failed to create data-dir %q: %w", a.dataDir, err)
	}

	conf, err := config.LoadConfiguration(a.flags.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration from %q: %w", a.flags.ConfigPath, err)
	}
	a.appConfig = conf
	a.applyFlagOverrides()
	logBoot("Headless mode: loaded config from %s", a.flags.ConfigPath)
	return nil
}

func (a *Application) loadManaged() error {
	absDir, err := filepath.Abs(a.flags.DataDir)
	if err != nil {
		return fmt.Errorf("failed to resolve data-dir %q: %w", a.flags.DataDir, err)
	}
	a.dataDir = absDir

	if err := os.MkdirAll(a.dataDir, 0750); err != nil {
		return fmt.Errorf("failed to create data-dir %q: %w", a.dataDir, err)
	}

	a.workingConfigPath = filepath.Join(a.dataDir, workingConfigName)

	_, statErr := os.Stat(a.workingConfigPath)
	workingExists := statErr == nil

	switch {
	case a.flags.ConfigPath != "" && !workingExists:
		// First run: seed the working config from --config
		if err := fsutil.CopyFile(a.flags.ConfigPath, a.workingConfigPath); err != nil {
			return fmt.Errorf("failed to seed working config from %q: %w", a.flags.ConfigPath, err)
		}
		logBoot("Seeded working config from %s → %s", a.flags.ConfigPath, a.workingConfigPath)

	case a.flags.ConfigPath != "" && workingExists:
		// Working config already exists — ignore --config, warn the user
		logBoot("Working config %s already exists, ignoring --config %s (delete %s to re-seed)",
			a.workingConfigPath, a.flags.ConfigPath, workingConfigName)

	case a.flags.ConfigPath == "" && workingExists:
		// Normal subsequent run
		logBoot("Loading existing working config from %s", a.workingConfigPath)

	default:
		// No --config and no working config
		return fmt.Errorf("no configuration found: provide --config to seed, or ensure %s exists in data-dir %s",
			workingConfigName, a.dataDir)
	}

	conf, err := config.LoadConfiguration(a.workingConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load working config from %q: %w", a.workingConfigPath, err)
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
	if a.flags.IsHeadless() {
		if a.flags.ConfigPath == "" {
			return fmt.Errorf("--config is required in headless mode")
		}
		return nil
	}
	// Managed mode validations
	if a.appConfig.APIServer.Port <= 0 || a.appConfig.APIServer.Port > 65535 {
		return fmt.Errorf("invalid API server port: %d", a.appConfig.APIServer.Port)
	}
	return nil
}

func (a *Application) Start() error {
	if a.flags.IsHeadless() {
		return a.startHeadless()
	}
	return a.startManaged()
}

func (a *Application) startHeadless() error {
	svc, err := app.NewHeadlessService(a.appConfig, a.dataDir)
	if err != nil {
		return fmt.Errorf("failed to create headless service: %w", err)
	}
	a.service = svc

	if err := a.service.Start(); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	zap.S().Info("Application started in headless mode (no UI)")
	return nil
}

func (a *Application) startManaged() error {
	svc, err := app.NewSystemService(a.appConfig, a.workingConfigPath, a.dataDir)
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
		broker := a.service.GetSSEBroker()
		if broker != nil {
			zap.S().Info("Closing SSE broker to release streaming connections")
			broker.Stop()
		}
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
