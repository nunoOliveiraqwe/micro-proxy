package torii

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/nunoOliveiraqwe/torii/config"
	"go.uber.org/zap"
)

type debugServers struct {
	portMap      map[string]bool
	httpServers  []*http.Server
	tcpListeners []net.Listener
}

func (d *debugServers) addHTTP(srv *http.Server) {
	d.httpServers = append(d.httpServers, srv)
}

func (d *debugServers) addTCP(ln net.Listener) {
	d.tcpListeners = append(d.tcpListeners, ln)
}

func (d *debugServers) Shutdown(ctx context.Context) {
	for _, srv := range d.httpServers {
		if err := srv.Shutdown(ctx); err != nil {
			zap.S().Errorf("Debug HTTP server shutdown error on %s: %v", srv.Addr, err)
		}
	}
	for _, ln := range d.tcpListeners {
		if err := ln.Close(); err != nil {
			zap.S().Errorf("Debug TCP listener close error on %s: %v", ln.Addr(), err)
		}
	}
}

// RunDebugMode starts lightweight stub servers for every configured backend so
// the proxy can be exercised end-to-end without real upstream services.
// The stubs listen on localhost using the port extracted from each backend
// address (the host part is ignored because backends might point to external
// services).
func (a *Application) RunDebugMode() {
	if a.flags == nil || a.flags.Debug == nil || !*a.flags.Debug {
		return
	}

	a.debug = &debugServers{
		portMap: make(map[string]bool),
	}

	zap.S().Info("Running in debug mode — proxied services will be stubbed locally")

	for _, ln := range a.appConfig.NetConfig.HTTPListeners {
		for _, backend := range httpListenerBackends(ln) {
			port, err := extractPort(backend)
			if err != nil {
				zap.S().Errorf("Failed to parse backend address %q for HTTP listener on port %d: %v", backend, ln.Port, err)
				continue
			}
			a.startDebugHTTPServer(port)
		}
	}

	for _, tln := range a.appConfig.NetConfig.TCPListeners {
		port, err := extractPort(tln.Backend)
		if err != nil {
			zap.S().Errorf("Failed to parse backend address %q for TCP listener on port %d: %v", tln.Backend, tln.Port, err)
			continue
		}
		a.startDebugTCPServer(port)
	}
}

func (a *Application) ShutdownDebug(ctx context.Context) {
	if a.debug == nil {
		return
	}
	zap.S().Info("Shutting down debug servers")
	a.debug.Shutdown(ctx)
}

func httpListenerBackends(ln config.HTTPListener) []string {
	if ln.Default != nil {
		return []string{ln.Default.Backend}
	}
	backends := make([]string, 0, len(ln.Routes))
	for _, r := range ln.Routes {
		backends = append(backends, r.Target.Backend)
	}
	return backends
}

// extractPort returns the port from a backend address. Supported formats:
//
//	http://localhost:8095   → 8095
//	localhost:8095          → 8095
//	127.0.0.1:8095          → 8095
//	https://example.com:8443 → 8443
//	http://90.90.90.90      → 80
//	https://90.90.90.90     → 443
func extractPort(backend string) (string, error) {
	if strings.Contains(backend, "://") {
		u, err := url.Parse(backend)
		if err != nil {
			return "", fmt.Errorf("invalid backend URL %q: %w", backend, err)
		}
		if p := u.Port(); p != "" {
			return p, nil
		}
		switch u.Scheme {
		case "http":
			return "80", nil
		case "https":
			return "443", nil
		default:
			return "", fmt.Errorf("no port and unknown scheme %q in backend %q", u.Scheme, backend)
		}
	}

	_, port, err := net.SplitHostPort(backend)
	if err != nil {
		return "", fmt.Errorf("invalid backend address %q: %w", backend, err)
	}
	return port, nil
}

func (a *Application) startDebugHTTPServer(port string) {
	addr := fmt.Sprintf(":%s", port)
	zap.S().Infof("Starting HTTP debug server on %s", addr)
	_, exists := a.debug.portMap[port]
	if exists {
		zap.S().Infof("Debug server already started on port %s", port)
		return
	}
	a.debug.portMap[port] = true
	srv := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Debug server running"))
		}),
	}
	a.debug.addHTTP(srv)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.S().Errorf("Debug HTTP server on %s failed: %v", addr, err)
		}
		delete(a.debug.portMap, port)
	}()
}

func (a *Application) startDebugTCPServer(port string) {
	addr := fmt.Sprintf(":%s", port)
	zap.S().Infof("Starting TCP debug server on %s", addr)
	_, exists := a.debug.portMap[port]
	if exists {
		zap.S().Infof("Debug server already started on port %s", port)
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		zap.S().Errorf("Failed to start debug TCP server on %s: %v", addr, err)
		return
	}
	a.debug.addTCP(ln)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Expected when the listener is closed during shutdown.
				delete(a.debug.portMap, port)
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte("Debug TCP server running\n"))
			}(conn)
		}
	}()
}
