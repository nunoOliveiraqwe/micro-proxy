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

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/fsutil"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy/acme"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
)

type ToriiHttpsServer struct {
	httpServer        *http.Server
	serverId          string
	handler           *SwappableHandler
	cancelChain       context.CancelFunc
	readTimeout       time.Duration
	readHeaderTimeout time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	isStarted         atomic.Bool
	bindPort          int
	iPV4BindInterface string
	iPV6BindInterface string
	useAcme           bool
	disableHTTP2      bool
	keyFilePath       string
	certFilepath      string
	middlewareChain   []string
	backends          []string
	routes            []RouteSnapshot
	errorMessage      string
	currentConfig     config.HTTPListener
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
		ErrorMessage:    m.errorMessage,
	}
}

func (m *ToriiHttpsServer) GetServerId() string {
	return m.serverId
}

func (m *ToriiHttpsServer) GetCurrentConfig() config.HTTPListener {
	return m.currentConfig
}

func (m *ToriiHttpsServer) DoesConfigChangeRequireServerRestart(newConf config.HTTPListener) bool {
	//Bind stack or interface change is a hard restart because we need to rebind the listeners
	if m.currentConfig.Bind != newConf.Bind || m.currentConfig.Interface != newConf.Interface {
		return true
	}
	//port rebind required restart
	if m.currentConfig.Port != newConf.Port {
		return true
	}
	//any time change requires server restart
	if m.currentConfig.ReadTimeout != newConf.ReadTimeout || m.currentConfig.ReadHeaderTimeout != newConf.ReadHeaderTimeout ||
		m.currentConfig.WriteTimeout != newConf.WriteTimeout || m.currentConfig.IdleTimeout != newConf.IdleTimeout {
		return true
	}
	//http2 is configured internally for TLS
	if newConf.DisableHTTP2 != m.currentConfig.DisableHTTP2 {
		return true
	}
	//ACME configuration change requires restart since it changes how TLS is configured
	newHasTLS := newConf.TLS != nil
	if !newHasTLS {
		//changes from HTTPS to HTTP
		return true
	}

	if m.currentConfig.TLS.UseAcme != newConf.TLS.UseAcme || m.currentConfig.TLS.Cert != newConf.TLS.Cert ||
		m.currentConfig.TLS.Key != newConf.TLS.Key {
		return true
	}

	return false
}

func (m *ToriiHttpsServer) start(acmeManager *acme.LegoAcmeManager) error {
	zap.S().Infof("Starting HTTPS server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
	listeners := buildNetListeners(m.iPV4BindInterface, m.iPV6BindInterface, m.bindPort)
	closeListeners := func() {
		for _, ln := range listeners {
			if err := ln.Close(); err != nil {
				zap.S().Warnf("Failed to close listener: %v", err)
			}
		}
	}
	success := false
	defer func() {
		if !success {
			zap.S().Warnf("HTTPS server failed to start, closing listeners")
			closeListeners()
		}
	}()
	numberOfRequiredListeners := 0
	if m.iPV4BindInterface != "" {
		numberOfRequiredListeners++
	}
	if m.iPV6BindInterface != "" {
		numberOfRequiredListeners++
	}
	if len(listeners) == 0 {
		zap.S().Errorf("No listeners available to start HTTPS server")
		m.errorMessage = fmt.Sprintf("No listeners available to start HTTPS server on port %d", m.bindPort)
		return fmt.Errorf("no listeners available for port %d", m.bindPort)
	} else if len(listeners) != numberOfRequiredListeners {
		zap.S().Errorf("Expected %d listeners based on configuration but got %d, cannot start HTTPS server", numberOfRequiredListeners, len(listeners))
		m.errorMessage = fmt.Sprintf("Expected %d listeners based on configuration but got %d, cannot start HTTPS server on port %d", numberOfRequiredListeners,
			len(listeners), m.bindPort)
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
			m.errorMessage = fmt.Sprintf("ACME is enabled but no ACME manager is configured for port %d", m.bindPort)
			return fmt.Errorf("ACME is enabled but no ACME manager is configured")

		}
		m.httpServer.TLSConfig = acmeManager.GetTLSConfig()
		if !m.disableHTTP2 {
			if err := http2.ConfigureServer(m.httpServer, nil); err != nil {
				m.errorMessage = fmt.Sprintf("failed to configure HTTP/2 for ACME server: %v", err)
				return fmt.Errorf("failed to configure HTTP/2 for ACME server: %w", err)
			}
		}
		m.isStarted.Store(true)
		m.errorMessage = ""
		success = true
		for _, listener := range listeners {
			tlsListener := tls.NewListener(listener, m.httpServer.TLSConfig)
			go func(ln net.Listener) {
				zap.S().Infof("Starting ACME HTTPS server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
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
		if m.disableHTTP2 {
			// ServeTLS auto-configures H2 unless TLSNextProto is a non-nil map.
			m.httpServer.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
		}
		m.isStarted.Store(true)
		m.errorMessage = ""
		success = true
		for _, listener := range listeners {
			go func(ln net.Listener) {
				zap.S().Infof("Starting HTTPS server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
				if err := m.httpServer.ServeTLS(ln, m.certFilepath, m.keyFilePath); err != nil && !errors.Is(err, http.ErrServerClosed) {
					zap.S().Errorf("Error serving HTTPS server: %v", err)
				}
			}(listener)
		}
		return nil
	}
	m.errorMessage = fmt.Sprintf("HTTPS server cannot start: no ACME manager and no valid certificate/key files provided for port %d", m.bindPort)
	return fmt.Errorf("HTTPS server cannot start: no ACME manager and no valid certificate/key files provided")
}

func (m *ToriiHttpsServer) stop() error {
	zap.S().Infof("Stopping HTTPS server")
	if m.cancelChain != nil {
		m.cancelChain()
	}
	if m.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := m.httpServer.Shutdown(ctx)
	m.isStarted.Store(false)
	return err
}

func (m *ToriiHttpsServer) getHandler() http.Handler {
	return m.handler.Load()
}

func (m *ToriiHttpsServer) updateHandler(handler http.Handler, cancel context.CancelFunc) error {
	if m.cancelChain != nil {
		m.cancelChain()
	}
	m.cancelChain = cancel
	old := m.handler.Swap(handler)
	if m.isStarted.Load() {
		zap.S().Infof("Hot-swapped HTTPS handler on port %d (was %T, now %T)", m.bindPort, old, handler)
	}
	return nil
}
