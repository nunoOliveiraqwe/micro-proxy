package acme

import (
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/lego"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"go.uber.org/zap"
)

type LegoAcmeManager struct {
	mu              sync.RWMutex
	store           store.AcmeStore
	client          *lego.Client
	user            *acmeUser
	certCache       map[string]*tls.Certificate
	conf            *domain.AcmeConfiguration
	domainSupplier  func() []string // callback to discover route domains from the proxy
	renewalInterval time.Duration
	stopCh          chan struct{}
}

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
		conf:            conf,
		certCache:       make(map[string]*tls.Certificate),
		renewalInterval: renewalInterval,
		stopCh:          make(chan struct{}),
	}
	registerLogger()
	if err := mgr.loadOrCreateAccount(); err != nil {
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
	if err := client.Challenge.SetDNS01Provider(provider); err != nil {
		return nil, fmt.Errorf("acme: set dns-01 provider: %w", err)
	}

	mgr.client = client

	if err := mgr.registerIfNeeded(); err != nil {
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
	m.StartRenewalLoop()
}

func (m *LegoAcmeManager) Stop() {
	select {
	case <-m.stopCh:
		// already closed
	default:
		close(m.stopCh)
	}
}
