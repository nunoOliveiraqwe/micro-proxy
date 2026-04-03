package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/nunoOliveiraqwe/torii/metrics"
	"github.com/nunoOliveiraqwe/torii/proxy/acme"
	"go.uber.org/zap"
)

type ToriiHttpServer struct {
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
	middlewareChain   []string
	backends          []string
	routes            []RouteSnapshot
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
	}
}

func (m *ToriiHttpServer) GetServerId() string {
	return m.serverId
}

func (m *ToriiHttpServer) start(_ *acme.LegoAcmeManager) error {
	zap.S().Infof("Starting HTTP server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
	listeners := buildNetListeners(m.iPV4BindInterface, m.iPV6BindInterface, m.bindPort)
	if len(listeners) == 0 {
		zap.S().Errorf("No listeners available to start HTTP server")
		return fmt.Errorf("no listeners available for port %d", m.bindPort)
	}
	m.httpServer = &http.Server{
		Handler:           m.handler,
		ReadTimeout:       m.readTimeout,
		ReadHeaderTimeout: m.readHeaderTimeout,
		WriteTimeout:      m.writeTimeout,
		IdleTimeout:       m.idleTimeout,
	}
	for _, listener := range listeners {
		go func(ln net.Listener) {
			m.isStarted.Store(true)
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
	if m.httpServer == nil {
		return nil
	}
	m.isStarted.Store(false)
	return m.httpServer.Shutdown(context.Background())
}

func (m *ToriiHttpServer) getHandler() http.Handler {
	return m.handler
}

func (m *ToriiHttpServer) updateHandler(handler http.Handler) error {
	if m.isStarted.Load() {
		return fmt.Errorf("HTTP server is already started, cannot update handler")
	}
	m.handler = handler
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
