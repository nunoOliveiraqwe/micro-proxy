package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"reflect"
	"sync"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/ctxkeys"
	"github.com/nunoOliveiraqwe/torii/internal/service"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"go.uber.org/zap"
)

var ErrProxyNotFound = fmt.Errorf("proxy does not exist")

type MicroHttpServer interface {
	GetServerId() string
	GetProxySnapshot(metric []*metrics.Metric) *ProxySnapshot
	GetCurrentConfig() config.HTTPListener
	DoesConfigChangeRequireServerRestart(newConf config.HTTPListener) bool
	start(tls *tls.Config) error
	getHandler() http.Handler
	updateHandler(handler http.Handler, cancel context.CancelFunc) error
	stop() error
}

type Torii struct {
	stoppedHttpServers map[int]MicroHttpServer
	startedHttpServers map[int]MicroHttpServer
	lock               sync.Mutex
	gDispatcher        *GlobalDispatcher
	acmeService        *service.AcmeService
	metricsManager     *metrics.ConnectionMetricsManager
	cacheManager       *util.CacheInsightManager
}

func NewTorii(conf config.NetworkConfig, mgr *metrics.ConnectionMetricsManager,
	cacheMgr *util.CacheInsightManager, acmeService *service.AcmeService) (*Torii, error) {
	zap.S().Info("Initializing torii with configuration: ", conf)
	m := Torii{
		stoppedHttpServers: make(map[int]MicroHttpServer),
		startedHttpServers: make(map[int]MicroHttpServer),
		lock:               sync.Mutex{},
		metricsManager:     mgr,
		acmeService:        acmeService,
		cacheManager:       cacheMgr,
	}
	ctx := context.WithValue(context.Background(), ctxkeys.MetricsMgr, mgr)
	ctx = context.WithValue(ctx, ctxkeys.CacheInsightMgr, cacheMgr)
	err := m.initializeHttpNetworkStackFromConf(ctx, conf)
	if err != nil {
		return nil, err
	}
	if acmeService != nil {
		zap.S().Info("ACME service provided, registering ACME proxy")
		acmeService.RegisterProxy(&service.AcmeRegisteredProxy{
			DomainSupplier: m.collectRouteDomains,
		})
	} else {
		zap.S().Info("No ACME service provided, skipping ACME proxy registration")
	}
	return &m, nil
}

func (m *Torii) StartAll() error {
	zap.S().Infof("Starting torii with %d HTTP servers", len(m.stoppedHttpServers))
	for port, _ := range m.stoppedHttpServers {
		err := m.StartHttpProxy(port)
		if err != nil {
			zap.S().Errorf("Failed to start HTTP server on port %d: %v", port, err)
		}
	}

	//TODO -> start TCP servers
	return nil
}

func (m *Torii) StopAll() error {
	zap.S().Infof("Stopping torii with %d HTTP servers", len(m.startedHttpServers))
	for port, _ := range m.startedHttpServers {
		err := m.StopHttpProxy(port)
		if err != nil {
			zap.S().Errorf("Failed to stop HTTP server on port %d: %v", port, err)
		}
	}
	//TODO -> stop TCP servers
	return nil
}

func (m *Torii) StartHttpProxy(port int) error {
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

	var tlsConf *tls.Config
	if m.acmeService != nil {
		tlsConf = m.acmeService.GetAcmeTLSConfig()
	}
	err := server.start(tlsConf)
	if err != nil {
		zap.S().Errorf("Failed to start HTTP server on port %d: %v", port, err)
		return err
	}
	m.startedHttpServers[port] = server
	delete(m.stoppedHttpServers, port)
	return nil
}

func (m *Torii) StopHttpProxy(port int) error {
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

func (m *Torii) DeleteHttpProxy(port int) error {
	zap.S().Infof("Deleting proxy server on port %d", port)
	if port <= 0 {
		return fmt.Errorf("invalid port number: %d", port)
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	_, isStopped := m.stoppedHttpServers[port]

	if isStopped {
		//server stopped, just delete
		zap.S().Warnf("Deleting stopped proxy server on port %d", port)
		delete(m.stoppedHttpServers, port)
		zap.S().Warnf("Deleted stopped proxy server on port %d", port)
		return nil
	}

	s, isStarted := m.startedHttpServers[port]

	if !isStarted {
		zap.S().Warnf("No started or stopped HTTP server found for port %d", port)
		return fmt.Errorf("no started or stopped HTTP server found for port %d", port)
	}
	zap.S().Infof("Stopping HTTP server on port %d, following with a deletetion", port)
	err := s.stop()
	if err != nil {
		zap.S().Errorf("Failed to stop HTTP server on port %d: %v", port, err)
		return err
	}
	zap.S().Warnf("Deleting stopped proxy server on port %d", port)
	delete(m.startedHttpServers, port)
	zap.S().Warnf("Deleted stopped proxy server on port %d", port)
	return nil
}

func (m *Torii) AddHttpServer(ctx context.Context, conf config.HTTPListener) error {
	zap.S().Debugf("Adding HTTP server for listener configuration: %+v", conf)
	m.lock.Lock()
	defer m.lock.Unlock()
	httpServer, err := buildHttpServer(ctx, conf, m.gDispatcher)
	if err != nil {
		zap.S().Errorf("Failed to build HTTP server: %v", err)
		return fmt.Errorf("failed to build HTTP server: %w", err)
	}
	m.stoppedHttpServers[conf.Port] = httpServer
	return nil
}

func (m *Torii) HotSwapHandler(ctx context.Context, port int, conf config.HTTPListener) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	server, ok := m.startedHttpServers[port]
	if !ok {
		server, ok = m.stoppedHttpServers[port]
	}
	if !ok {
		return fmt.Errorf("no HTTP server found for port %d", port)
	}

	//skip the swap entirely if the config hasn't changed.
	currentConfig := server.GetCurrentConfig()
	if reflect.DeepEqual(currentConfig, conf) {
		zap.S().Infof("Config unchanged for port %d, skipping hot-swap", port)
		return nil
	}

	ctx = context.WithValue(ctx, ctxkeys.MetricsMgr, m.metricsManager)
	ctx = context.WithValue(ctx, ctxkeys.CacheInsightMgr, m.cacheManager)

	//TODO -> optimize by only rebuilding the affected route's handler's instead of the whole handler chain.
	//only swap what's changed
	handler, cancel, backends, routeSnapshots, err := buildHandlerChain(ctx, server.GetServerId(), conf, m.gDispatcher)
	if err != nil {
		return fmt.Errorf("failed to build handler chain: %w", err)
	}

	if err := server.updateHandler(handler, cancel); err != nil {
		cancel()
		return fmt.Errorf("failed to swap handler: %w", err)
	}

	m.updateServerMetadata(server, backends, routeSnapshots, conf)

	zap.S().Infof("Hot-swapped handler for server on port %d", port)
	return nil
}

func (m *Torii) AddTcpServer(conf config.TCPListener) {
	//TODO
}

func (m *Torii) GetProxyConfSnapshots() []*ProxySnapshot {
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

func (m *Torii) GetProxyConfig(port int) *config.HTTPListener {
	m.lock.Lock()
	defer m.lock.Unlock()
	if server, ok := m.stoppedHttpServers[port]; ok {
		c := server.GetCurrentConfig()
		return &c
	}
	if server, ok := m.startedHttpServers[port]; ok {
		c := server.GetCurrentConfig()
		return &c
	}
	return nil
}

func (m *Torii) GetAllHTTPConfigs() []config.HTTPListener {
	m.lock.Lock()
	defer m.lock.Unlock()
	var configs []config.HTTPListener
	for _, server := range m.stoppedHttpServers {
		c := server.GetCurrentConfig()
		configs = append(configs, c)
	}
	for _, server := range m.startedHttpServers {
		c := server.GetCurrentConfig()
		configs = append(configs, c)
	}
	return configs
}

func (m *Torii) IsStarted(port int) bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	_, ok := m.startedHttpServers[port]
	return ok
}

func (m *Torii) DoesConfigRequireServerRestart(oldPort int, newConf config.HTTPListener) (bool, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	server, ok := m.startedHttpServers[oldPort]
	if !ok {
		server, ok = m.stoppedHttpServers[oldPort]
	}
	if !ok {
		return false, ErrProxyNotFound
	}
	return server.DoesConfigChangeRequireServerRestart(newConf), nil
}

func (m *Torii) initializeHttpNetworkStackFromConf(ctx context.Context, conf config.NetworkConfig) error {
	zap.S().Debugf("Initializing HTTP network stack with configuration: %+v", conf)

	if conf.Global != nil {
		zap.S().Debugf("Initializing global dispatcher with configuration: %+v", conf.Global)
		dGlobal, err := initGlobalDispatcher(ctx, conf.Global)
		if err != nil {
			return fmt.Errorf("failed to initialize global dispatcher: %w", err)
		}
		m.gDispatcher = dGlobal
	}

	zap.S().Infof("Initializing HTTP servers")
	if (conf.HTTPListeners == nil || len(conf.HTTPListeners) == 0) &&
		(len(conf.TCPListeners) == 0 || conf.TCPListeners == nil) &&
		(conf.Global == nil) {
		zap.S().Warn("No network configurations provided, skipping server initialization")
		return nil
	}
	for _, ln := range conf.HTTPListeners {
		zap.S().Debugf("Initializing HTTP server with configuration: %+v", ln)
		m.AddHttpServer(ctx, ln)
	}
	return nil
}

func (m *Torii) collectRouteDomains() []string {
	m.lock.Lock()
	defer m.lock.Unlock()
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

		// Only collect domains from ACME-enabled HTTPS servers.
		httpsServer, ok := server.(*ToriiHttpsServer)
		if !ok || !httpsServer.useAcme {
			continue
		}

		handler := server.getHandler()

		if handler == nil {
			zap.S().Warnf("Found nil httpServerHandler, skipping")
			continue
		}

		if dispatcher, ok := handler.(*VirtualHostDispatcher); ok {
			domains = append(domains, dispatcher.routeTrie.GetAllHosts()...)
		}
	}
	return domains
}

func (m *Torii) updateServerMetadata(server MicroHttpServer, backends []string, routes []RouteSnapshot, conf config.HTTPListener) {
	mwNames := collectMiddlewareNames(routes)
	switch s := server.(type) {
	case *ToriiHttpServer:
		s.middlewareChain = mwNames
		s.backends = backends
		s.routes = routes
		s.currentConfig = conf
	case *ToriiHttpsServer:
		s.middlewareChain = mwNames
		s.backends = backends
		s.routes = routes
		s.currentConfig = conf
	}
}
