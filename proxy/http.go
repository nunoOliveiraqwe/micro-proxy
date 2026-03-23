package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/nunoOliveiraqwe/micro-proxy/metrics"
	"go.uber.org/zap"
)

type MicroProxyHttpServer struct {
	httpServer        *http.Server
	isStarted         atomic.Bool
	bindPort          int
	iPV4BindInterface string
	iPV6BindInterface string
	middlewareChain   []string
	metricsName       string
}

func (m *MicroProxyHttpServer) GetProxySnapshot() *ProxySnapshot {
	return &ProxySnapshot{
		Port:            m.bindPort,
		Interface:       fmt.Sprintf("ipv4=%s, ipv6=%s", m.iPV4BindInterface, m.iPV6BindInterface),
		MiddlewareChain: m.middlewareChain,
		IsStarted:       m.isStarted.Load(),
		IsUsingHTTPS:    false,
		IsUsingACME:     false,
		Metric:          metrics.GlobalMetricsManager.GetMetricForConnection(m.metricsName),
	}
}

func (m *MicroProxyHttpServer) start(_ *MicroProxyAcmeManager) error {
	zap.S().Infof("Starting HTTP server on %d, ipv4 = %s, ipv6 = %s", m.bindPort, m.iPV4BindInterface, m.iPV6BindInterface)
	listeners := buildNetListeners(m.iPV4BindInterface, m.iPV6BindInterface, m.bindPort)
	if len(listeners) == 0 {
		zap.S().Errorf("No listeners available to start HTTP server")
		return nil
	}
	for _, listener := range listeners {
		go func(ln net.Listener) {
			m.isStarted.Store(true)
			if err := m.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				zap.S().Errorf("HTTP server error: %v", err)
				err := listener.Close()
				if err != nil {
					zap.S().Errorf("Failed to close listener: %v", err)
					return
				}
			}
		}(listener)
	}
	return nil
}

func (m *MicroProxyHttpServer) stop() error {
	zap.S().Infof("Stopping HTTP server")
	if m.httpServer == nil {
		return nil
	}
	m.isStarted.Store(false)
	return m.httpServer.Shutdown(context.Background())
}

func (m *MicroProxyHttpServer) getHandler() http.Handler {
	return m.httpServer.Handler
}

func (m *MicroProxyHttpServer) updateHandler(handler http.Handler) error {
	if m.isStarted.Load() {
		return fmt.Errorf("HTTP server is already started, cannot update handler")
	}
	m.httpServer.Handler = handler
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
			lns = append(lns, listen)
		}
	}
	if ipv6BindIf != "" {
		addr := fmt.Sprintf("%s:%d", ipv6BindIf, port)
		listen, err := net.Listen("tcp6", addr)
		if err != nil {
			zap.S().Errorf("Failed to listen on IPv6 interface: %s", err)
		} else {
			lns = append(lns, listen)
		}
	}
	return lns
}
