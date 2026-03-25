package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/nunoOliveiraqwe/micro-proxy/config"
	"github.com/nunoOliveiraqwe/micro-proxy/metrics"
	"go.uber.org/zap"
)

type MicroHttpServer interface {
	GetServerId() string
	GetProxySnapshot(metric []*metrics.Metric) *ProxySnapshot
	start(acmeManager *MicroProxyAcmeManager) error
	getHandler() http.Handler
	updateHandler(handler http.Handler) error
	stop() error
}

type MicroProxy struct {
	stoppedHttpServers map[int]MicroHttpServer
	startedHttpServers map[int]MicroHttpServer
	lock               sync.Mutex
	acmeManager        *MicroProxyAcmeManager
	metricsManager     *metrics.ConnectionMetricsManager
}

func NewMicroProxy(conf config.NetworkConfig, mgr *metrics.ConnectionMetricsManager) (*MicroProxy, error) {
	zap.S().Info("Initializing MicroProxy with configuration: ", conf)
	m := MicroProxy{
		stoppedHttpServers: make(map[int]MicroHttpServer),
		startedHttpServers: make(map[int]MicroHttpServer),
		lock:               sync.Mutex{},
		metricsManager:     mgr,
	}
	ctx := context.WithValue(context.Background(), "metricsManager", mgr)
	err := m.initializeHttpNetworkStackFromConf(ctx, conf)
	if err != nil {
		return nil, err
	}
	if conf.ACMEConfig != nil {
		err = m.StartAcmeManager(conf.ACMEConfig)
		if err != nil {
			return nil, err
		}
	}
	return &m, nil
}

func (m *MicroProxy) StartAll() error {
	zap.S().Infof("Starting MicroProxy with %d HTTP servers", len(m.stoppedHttpServers))
	for port, _ := range m.stoppedHttpServers {
		//hmmm, i can't lock here, because i wanto to call start
		err := m.StartProxy(port)
		if err != nil {
			zap.S().Errorf("Failed to start HTTP server on port %d: %v", port, err)
			//should i stop everything if one fails??
		}
	}

	//TODO -> start TCP servers
	return nil
}

func (m *MicroProxy) StopAll() error {
	zap.S().Infof("Stopping MicroProxy with %d HTTP servers", len(m.startedHttpServers))
	for port, _ := range m.startedHttpServers {
		err := m.StopProxy(port)
		if err != nil {
			zap.S().Errorf("Failed to stop HTTP server on port %d: %v", port, err)
		}
	}
	//TODO -> stop TCP servers
	return nil
}

func (m *MicroProxy) StartProxy(port int) error {
	zap.S().Infof("Starting proxy server on port %d", port)
	if port <= 0 {
		return fmt.Errorf("invalid port number: %d", port)
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.startedHttpServers[port] != nil {
		return fmt.Errorf("proxy server already started on port %d", port)
	}
	server, ok := m.stoppedHttpServers[port]
	if !ok {
		return fmt.Errorf("no stopped HTTP server found for port %d", port)
	}
	if port == 80 && m.acmeManager != nil { //handle por 80 attach
		zap.S().Infof("Attaching ACME manager to port 80")
		serverHandler := server.getHandler()
		if serverHandler != nil {
			zap.S().Debugf("Original handler for port 80: %T", serverHandler)
			nHandler := m.acmeManager.bindAcmeHandlerToPort80(serverHandler)
			if nHandler != nil {
				err := server.updateHandler(nHandler)
				if err != nil {
					zap.S().Errorf("Failed to update handler for port 80 with ACME manager: %v", err)
					return err
				}
			}
		}
	}

	err := server.start(m.acmeManager)
	if err != nil {
		zap.S().Errorf("Failed to start HTTP server on port %d: %v", port, err)
		return err
	}
	m.startedHttpServers[port] = server
	delete(m.stoppedHttpServers, port)
	return nil
}

func (m *MicroProxy) StopProxy(port int) error {
	zap.S().Infof("Stopping proxy server on port %d", port)
	if port <= 0 {
		return fmt.Errorf("invalid port number: %d", port)
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.stoppedHttpServers[port] != nil {
		zap.S().Warnf("Proxy server already stopped on port %d", port)
		return fmt.Errorf("proxy server already stopped on port %d", port)
	}
	server, ok := m.startedHttpServers[port]
	if !ok {
		zap.S().Warnf("No started HTTP server found for port %d", port)
		return fmt.Errorf("no started HTTP server found for port %d", port)
	}
	zap.S().Infof("Stopping HTTP server on port %d", port)
	err := server.stop()
	if err != nil {
		zap.S().Errorf("Failed to stop HTTP server on port %d: %v", port, err)
		return err
	}
	m.stoppedHttpServers[port] = server
	delete(m.startedHttpServers, port)
	return nil
}

func (m *MicroProxy) StartAcmeManager(conf *config.ACMEConfig) error {
	if conf == nil {
		zap.S().Warnf("ACME configuration is nil, skipping ACME manager setup")
		return fmt.Errorf("ACME configuration is nil")
	} else if m.acmeManager != nil {
		zap.S().Warnf("ACME manager already initialized")
		return fmt.Errorf("ACME manager already initialized")
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	domains := m.collectWorkingDomains()
	m.acmeManager = newMicroProxyAcmeManager(domains, conf.Email, conf.Cache)
	return nil
}

func (m *MicroProxy) AddHttpServer(conf config.HTTPListener, mgr *metrics.ConnectionMetricsManager) error {
	zap.S().Debugf("Adding HTTP server for listener configuration: %+v", conf)
	m.lock.Lock()
	defer m.lock.Unlock()
	httpServer, err := buildHttpServer(conf, mgr)
	if err != nil {
		zap.S().Errorf("Failed to build HTTP server: %v", err)
		return fmt.Errorf("failed to build HTTP server: %w", err)
	}
	m.stoppedHttpServers[conf.Port] = httpServer
	return nil
}

func (m *MicroProxy) AddTcpServer(conf config.TCPListener) {
	//TODO
}

func (m *MicroProxy) GetProxyConfSnapshots() []*ProxySnapshot {
	proxySnapshots := make([]*ProxySnapshot, 0)
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, server := range m.stoppedHttpServers {
		serverId := server.GetServerId()
		zap.S().Debugf("Fetching all metrics for serverId %d", serverId)
		mts := m.metricsManager.GetAllMetricsByServer(serverId)
		proxySnapshots = append(proxySnapshots, server.GetProxySnapshot(mts))
	}
	for _, server := range m.startedHttpServers {
		serverId := server.GetServerId()
		zap.S().Debugf("Fetching all metrics for serverId %d", serverId)
		mts := m.metricsManager.GetAllMetricsByServer(serverId)
		proxySnapshots = append(proxySnapshots, server.GetProxySnapshot(mts))
	}
	return proxySnapshots
}

func (m *MicroProxy) initializeHttpNetworkStackFromConf(conf config.NetworkConfig, mgr *metrics.ConnectionMetricsManager) error {
	zap.S().Infof("Initializing HTTP servers")
	if conf.HTTPListeners == nil || len(conf.HTTPListeners) == 0 {
		zap.S().Warn("No HTTP network configurations provided, skipping server initialization")
		return nil
	}
	for _, ln := range conf.HTTPListeners {
		zap.S().Debugf("Initializing HTTP server with configuration: %+v", ln)
		m.AddHttpServer(ln, mgr)
	}
	return nil
}

func (m *MicroProxy) collectWorkingDomains() []string {
	zap.S().Debugf("Collecting working domains from HTTP servers")
	domains := make([]string, 0)
	domains = appendDomainsFromServers(m.stoppedHttpServers, domains)
	domains = appendDomainsFromServers(m.startedHttpServers, domains)
	return domains
}

func appendDomainsFromServers(servers map[int]MicroHttpServer, domains []string) []string {
	for _, server := range servers {
		if server == nil {
			zap.S().Warnf("Found nil server, skipping")
			continue
		}
		handler := server.getHandler()

		if handler == nil {
			zap.S().Warnf("Found nil httpServerHandler, skipping")
			continue
		}

		if dispatcher, ok := handler.(*MultiRouteHttpDispatcher); ok {
			for host := range dispatcher.routes {
				domains = append(domains, host)
			}
		}
	}
	return domains
}
