package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy/acme"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type ToriiHttpServer struct {
	httpServer *http.Server
	serverId   string
	handler    *SwappableHandler

	cancelChain       context.CancelFunc
	readTimeout       time.Duration
	readHeaderTimeout time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	isStarted         atomic.Bool
	bindPort          int
	iPV4BindInterface string
	iPV6BindInterface string
	disableH2C        bool
	middlewareChain   []string
	backends          []string
	routes            []RouteSnapshot
	errorMessage      string
	currentConfig     config.HTTPListener
}

func (m *ToriiHttpServer) GetProxySnapshot(metrics []*metrics.Metric) *ProxySnapshot {
	return &ProxySnapshot{
		Port:            m.bindPort,
		Interface:       fmt.Sprintf("ipv4=%s, ipv6=%s", m.iPV4BindInterface, m.iPV6BindInterface),
		MiddlewareChain: m.middlewareChain,
		IsStarted:       m.isStarted.Load(),
		IsUsingHTTPS:    false,
		IsUsingACME:     false,
		Metrics:         metrics,
		Backends:        m.backends,
		Routes:          m.routes,
		ErrorMessage:    m.errorMessage,
	}
}

func (m *ToriiHttpServer) GetServerId() string {
	return m.serverId
}

func (m *ToriiHttpServer) GetCurrentConfig() config.HTTPListener {
	return m.currentConfig
}

func (m *ToriiHttpServer) DoesConfigChangeRequireServerRestart(newConf config.HTTPListener) bool {

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

	if newConf.TLS != nil {
		//HTTP to HTTPS
		return true
	}

	return false
}

func (m *ToriiHttpServer) start(_ *acme.LegoAcmeManager) error {
	zap.S().Infof("Starting HTTP server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
	listeners := buildNetListeners(m.iPV4BindInterface, m.iPV6BindInterface, m.bindPort)
	closeListeners := func() {
		for _, ln := range listeners {
			if err := ln.Close(); err != nil {
				zap.S().Warnf("Failed to close listener: %v", err)
			}
		}
	}
	numberOfRequiredListeners := 0
	if m.iPV4BindInterface != "" {
		numberOfRequiredListeners++
	}
	if m.iPV6BindInterface != "" {
		numberOfRequiredListeners++
	}
	if len(listeners) == 0 {
		zap.S().Errorf("No listeners available to start HTTP server")
		m.errorMessage = fmt.Sprintf("No listeners available to start HTTP server on port %d", m.bindPort)
		return fmt.Errorf("no listeners available for port %d", m.bindPort)
	} else if len(listeners) != numberOfRequiredListeners {
		zap.S().Errorf("Expected %d listeners based on configuration but got %d, cannot start HTTP server", numberOfRequiredListeners, len(listeners))
		closeListeners()
		m.errorMessage = fmt.Sprintf("Expected %d listeners based on configuration but got %d, cannot start HTTP server on port %d",
			numberOfRequiredListeners, len(listeners), m.bindPort)
		return fmt.Errorf("expected %d listeners based on configuration but got %d", numberOfRequiredListeners, len(listeners))
	}

	var handler http.Handler = m.handler
	if !m.disableH2C {
		handler = h2c.NewHandler(m.handler, &http2.Server{})
	}

	m.httpServer = &http.Server{
		Handler:           handler,
		ReadTimeout:       m.readTimeout,
		ReadHeaderTimeout: m.readHeaderTimeout,
		WriteTimeout:      m.writeTimeout,
		IdleTimeout:       m.idleTimeout,
	}
	m.isStarted.Store(true)
	m.errorMessage = ""
	for _, listener := range listeners {
		go func(ln net.Listener) {
			if err := m.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				zap.S().Errorf("HTTP server error: %v", err)
				if err := ln.Close(); err != nil {
					zap.S().Errorf("Failed to close listener: %v", err)
				}
			}
		}(listener)
	}
	return nil
}

func (m *ToriiHttpServer) stop() error {
	zap.S().Infof("Stopping HTTP server")
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

func (m *ToriiHttpServer) getHandler() http.Handler {
	return m.handler.Load()
}

func (m *ToriiHttpServer) updateHandler(handler http.Handler, cancel context.CancelFunc) error {
	if m.cancelChain != nil {
		m.cancelChain()
	}
	m.cancelChain = cancel
	old := m.handler.Swap(handler)
	if m.isStarted.Load() {
		zap.S().Infof("Hot-swapped HTTP handler on port %d (was %T, now %T)", m.bindPort, old, handler)
	}
	return nil
}

func buildNetListeners(ipv4BindIf, ipv6BindIf string, port int) []net.Listener {
	zap.S().Infof("Building net listeners for IPv4 interface: %s", ipv4BindIf)
	zap.S().Infof("Building net listeners for IPv6 interface: %s", ipv6BindIf)
	lns := make([]net.Listener, 0, 2)
	if ipv4BindIf != "" {
		addr := fmt.Sprintf("%s:%d", ipv4BindIf, port)
		listen, err := net.Listen("tcp4", addr)
		if err != nil {
			zap.S().Errorf("Failed to listen on IPv4 interface: %s", err)
		} else {
			zap.S().Infof("Successfully bound to IPv4 interface: %s", addr)
			lns = append(lns, listen)
		}
	}
	if ipv6BindIf != "" {
		addr := fmt.Sprintf("[%s]:%d", ipv6BindIf, port)
		listen, err := net.Listen("tcp6", addr)
		if err != nil {
			zap.S().Errorf("Failed to listen on IPv6 interface: %s", err)
		} else {
			zap.S().Infof("Successfully bound to IPv6 interface: %s", addr)
			lns = append(lns, listen)
		}
	}
	return lns
}
