package acme

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"go.uber.org/zap"
)

type LegoAcmeManager struct {
	mu        sync.RWMutex
	certCache map[string]*tls.Certificate

	store store.AcmeStore

	configID int

	client *lego.Client
	user   *acmeUser

	domainSupplier  func() []string // callback to discover route domains from the proxy
	renewalInterval time.Duration

	startStopMu sync.Mutex
	cancelFunc  context.CancelFunc
}

func (m *LegoAcmeManager) ConfigID() int { return m.configID }

func NewLegoAcmeManager(conf *domain.AcmeConfiguration, acmeStore store.AcmeStore) (*LegoAcmeManager, error) {
	if conf.Email == "" {
		return nil, fmt.Errorf("acme: email is required")
	}
	if conf.DNSProvider == "" {
		return nil, fmt.Errorf("acme: dns-provider is required")
	}

	renewalInterval := conf.RenewalCheckInterval
	if renewalInterval <= 0 {
		renewalInterval = 12 * time.Hour
	}

	mgr := &LegoAcmeManager{
		store:           acmeStore,
		configID:        conf.ID,
		certCache:       make(map[string]*tls.Certificate),
		renewalInterval: renewalInterval,
	}
	registerLogger()
	if err := mgr.loadOrCreateAccount(conf); err != nil {
		return nil, fmt.Errorf("acme: account init: %w", err)
	}
	legoCfg := lego.NewConfig(mgr.user)
	if conf.CADirURL != "" {
		legoCfg.CADirURL = conf.CADirURL
	}
	legoCfg.Certificate.KeyType = certcrypto.EC256
	client, err := lego.NewClient(legoCfg)
	if err != nil {
		return nil, fmt.Errorf("acme: lego client: %w", err)
	}

	// DNS-01 challenge provider from registry
	factory, err := GetDNSProvider(conf.DNSProvider)
	if err != nil {
		return nil, fmt.Errorf("acme: %w", err)
	}

	provider, err := factory.Create(conf.SerializedFields)
	if err != nil {
		return nil, fmt.Errorf("acme: dns provider %q: %w", conf.DNSProvider, err)
	}
	var dnsOpts []dns01.ChallengeOption
	dnsResolvers := NormalizeDNSResolvers(conf.DNSResolvers)
	if len(dnsResolvers) > 0 {
		zap.S().Infof("acme: using recursive DNS resolvers: %v", dnsResolvers)
		dnsOpts = append(dnsOpts, dns01.AddRecursiveNameservers(dnsResolvers))
	}
	if err := client.Challenge.SetDNS01Provider(provider, dnsOpts...); err != nil {
		return nil, fmt.Errorf("acme: set dns-01 provider: %w", err)
	}

	mgr.client = client

	if err := mgr.registerIfNeeded(conf); err != nil {
		return nil, fmt.Errorf("acme: registration: %w", err)
	}

	if err := mgr.loadCertificatesFromStore(); err != nil {
		zap.S().Warnf("acme: could not warm cert cache from DB: %v", err)
	}

	return mgr, nil
}

func (m *LegoAcmeManager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

func (m *LegoAcmeManager) Start() {
	m.startStopMu.Lock()
	defer m.startStopMu.Unlock()

	if m.cancelFunc != nil {
		zap.S().Warn("Superfluous call to LegoAcmeManager.Start() ignored")
		return // already started
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	m.cancelFunc = cancelFunc
	m.startRenewalLoop(ctx)
}

func (m *LegoAcmeManager) Stop() {
	m.startStopMu.Lock()
	defer m.startStopMu.Unlock()

	if m.cancelFunc == nil {
		zap.S().Warn("Superfluous call to LegoAcmeManager.Stop() ignored")
		return // already stopped
	}
	m.cancelFunc()
	m.cancelFunc = nil
}

func (m *LegoAcmeManager) IsStarted() bool {
	m.startStopMu.Lock()
	defer m.startStopMu.Unlock()

	return m.cancelFunc != nil
}
