package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/fsutil"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy/acme"
	"go.uber.org/zap"
)

type ToriiHttpsServer struct {
	httpServer        *http.Server
	serverId          string
	handler           http.Handler
	readTimeout       time.Duration
	readHeaderTimeout time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	isStarted         atomic.Bool
	bindPort          int
	iPV4BindInterface string
	iPV6BindInterface string
	useAcme           bool
	keyFilePath       string
	certFilepath      string
	middlewareChain   []string
	backends          []string
	routes            []RouteSnapshot
}

func (m *ToriiHttpsServer) GetProxySnapshot(metrics []*metrics.Metric) *ProxySnapshot {
	return &ProxySnapshot{
		Port:            m.bindPort,
		Interface:       fmt.Sprintf("ipv4=%s, ipv6=%s", m.iPV4BindInterface, m.iPV6BindInterface),
		MiddlewareChain: m.middlewareChain,
		IsStarted:       m.isStarted.Load(),
		IsUsingHTTPS:    true,
		IsUsingACME:     m.useAcme,
		Metrics:         metrics,
		Backends:        m.backends,
		Routes:          m.routes,
	}
}

func (m *ToriiHttpsServer) GetServerId() string {
	return m.serverId
}

func (m *ToriiHttpsServer) start(acmeManager *acme.LegoAcmeManager) error {
	zap.S().Infof("Starting HTTPS server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
	listeners := buildNetListeners(m.iPV4BindInterface, m.iPV6BindInterface, m.bindPort)
	numberOfRequiredListeners := 0
	if m.iPV4BindInterface != "" {
		numberOfRequiredListeners++
	}
	if m.iPV6BindInterface != "" {
		numberOfRequiredListeners++
	}
	if len(listeners) == 0 {
		zap.S().Errorf("No listeners available to start HTTPS server")
		return fmt.Errorf("no listeners available for port %d", m.bindPort)
	} else if len(listeners) != numberOfRequiredListeners {
		zap.S().Errorf("Expected %d listeners based on configuration but got %d, cannot start HTTPS server", numberOfRequiredListeners, len(listeners))
		return fmt.Errorf("expected %d listeners based on configuration but got %d", numberOfRequiredListeners, len(listeners))
	}
	m.httpServer = &http.Server{
		Handler:           m.handler,
		ReadTimeout:       m.readTimeout,
		ReadHeaderTimeout: m.readHeaderTimeout,
		WriteTimeout:      m.writeTimeout,
		IdleTimeout:       m.idleTimeout,
	}
	if m.useAcme {
		zap.S().Infof("Starting ACME HTTPS server")
		if acmeManager == nil {
			return fmt.Errorf("ACME is enabled but no ACME manager is configured")
		}
		m.httpServer.TLSConfig = acmeManager.GetTLSConfig()
		for _, listener := range listeners {
			tlsListener := tls.NewListener(listener, m.httpServer.TLSConfig)
			go func(ln net.Listener) {
				zap.S().Infof("Starting ACME HTTPS server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
				m.isStarted.Store(true)
				if err := m.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					zap.S().Errorf("Error serving ACME HTTPS server: %v", err)
				}
			}(tlsListener)
		}
		return nil
	}
	if fsutil.FileExists(m.keyFilePath) && fsutil.FileExists(m.certFilepath) {
		zap.S().Infof("Starting HTTPS server with provided certificate and key")
		m.httpServer.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		for _, listener := range listeners {
			go func(ln net.Listener) {
				zap.S().Infof("Starting HTTPS server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
				m.isStarted.Store(true)
				if err := m.httpServer.ServeTLS(ln, m.certFilepath, m.keyFilePath); err != nil && !errors.Is(err, http.ErrServerClosed) {
					zap.S().Errorf("Error serving HTTPS server: %v", err)
				}
			}(listener)
		}
		return nil
	}
	return fmt.Errorf("HTTPS server cannot start: no ACME manager and no valid certificate/key files provided")
}

func (m *ToriiHttpsServer) stop() error {
	zap.S().Infof("Stopping HTTP server")
	if m.httpServer == nil {
		return nil
	}
	m.isStarted.Store(false)
	return m.httpServer.Shutdown(context.Background())
}

func (m *ToriiHttpsServer) getHandler() http.Handler {
	return m.handler
}

func (m *ToriiHttpsServer) updateHandler(handler http.Handler) error {
	if m.isStarted.Load() {
		return fmt.Errorf("HTTPS server is already started, cannot update handler")
	}
	m.handler = handler
	return nil
}
