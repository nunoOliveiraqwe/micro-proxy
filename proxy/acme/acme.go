package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"go.uber.org/zap"
)

type acmeUser struct {
	email        string
	registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

type LegoAcmeManager struct {
	mu              sync.RWMutex
	store           store.AcmeStore
	client          *lego.Client
	user            *acmeUser
	certCache       map[string]*tls.Certificate
	conf            *domain.AcmeConfiguration
	domains         []string
	renewalInterval time.Duration
	stopCh          chan struct{}
}

// NewLegoAcmeManager creates a fully-initialised ACME manager.
// It loads (or generates) an account key, sets up a lego client with the
// requested DNS provider and pre-populates the in-memory certificate cache
// from the database.
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

func (m *LegoAcmeManager) loadOrCreateAccount() error {
	existing, err := m.store.GetAccount(m.conf.Email)
	if err != nil {
		return fmt.Errorf("store lookup: %w", err)
	}
	if existing != nil {
		key, err := pemToECKey(existing.PrivateKey)
		if err != nil {
			return fmt.Errorf("parse stored key: %w", err)
		}
		var reg registration.Resource
		if existing.Registration != "" {
			if err := json.Unmarshal([]byte(existing.Registration), &reg); err != nil {
				return fmt.Errorf("parse stored registration: %w", err)
			}
		}
		m.user = &acmeUser{
			email:        existing.Email,
			registration: &reg,
			key:          key,
		}
		zap.S().Infof("acme: loaded existing account for %s", existing.Email)
		return nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	m.user = &acmeUser{email: m.conf.Email, key: key}
	zap.S().Infof("acme: generated new account key for %s", m.conf.Email)
	return nil
}

func (m *LegoAcmeManager) registerIfNeeded() error {
	if m.user.registration != nil && m.user.registration.URI != "" {
		return nil // already registered
	}

	reg, err := m.client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	m.user.registration = reg

	if err := m.persistAccount(); err != nil {
		return fmt.Errorf("persist account: %w", err)
	}
	zap.S().Infof("acme: registered new account for %s (URI: %s)", m.conf.Email, reg.URI)
	return nil
}

func (m *LegoAcmeManager) persistAccount() error {
	ecKey, ok := m.user.key.(*ecdsa.PrivateKey)
	if !ok {
		return fmt.Errorf("account key is not ECDSA")
	}
	keyPEM, err := ecKeyToPEM(ecKey)
	if err != nil {
		return err
	}
	regJSON, err := json.Marshal(m.user.registration)
	if err != nil {
		return fmt.Errorf("marshal registration: %w", err)
	}
	return m.store.SaveAccount(&domain.AcmeAccount{
		Email:        m.conf.Email,
		PrivateKey:   keyPEM,
		Registration: string(regJSON),
	})
}

func (m *LegoAcmeManager) loadCertificatesFromStore() error {
	certs, err := m.store.ListCertificates()
	if err != nil {
		return err
	}
	for _, c := range certs {
		tlsCert, err := tls.X509KeyPair(c.Certificate, c.PrivateKey)
		if err != nil {
			zap.S().Warnf("acme: skip cached cert for %s: %v", c.Domain, err)
			continue
		}
		m.certCache[c.Domain] = &tlsCert
		zap.S().Infof("acme: cached cert for %s (expires %s)", c.Domain, c.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}

func (m *LegoAcmeManager) ObtainCertificate(domains []string) error {
	zap.S().Infof("acme: obtaining certificate for %v", domains)
	res, err := m.client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	})
	if err != nil {
		return fmt.Errorf("obtain: %w", err)
	}

	tlsCert, err := tls.X509KeyPair(res.Certificate, res.PrivateKey)
	if err != nil {
		return fmt.Errorf("parse obtained cert: %w", err)
	}
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse leaf: %w", err)
	}

	m.mu.Lock()
	for _, d := range domains {
		m.certCache[d] = &tlsCert
	}
	m.mu.Unlock()

	// Persist per-domain so each SNI name resolves independently.
	for _, d := range domains {
		if err := m.store.SaveCertificate(&domain.AcmeCertificate{
			Domain:            d,
			Certificate:       res.Certificate,
			PrivateKey:        res.PrivateKey,
			IssuerCertificate: res.IssuerCertificate,
			ExpiresAt:         leaf.NotAfter,
		}); err != nil {
			zap.S().Errorf("acme: persist cert for %s: %v", d, err)
		}
	}

	zap.S().Infof("acme: obtained cert for %v (expires %s)", domains, leaf.NotAfter.Format(time.RFC3339))
	return nil
}

func (m *LegoAcmeManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.RLock()
	cert, ok := m.certCache[hello.ServerName]
	m.mu.RUnlock()
	if ok {
		return cert, nil
	}
	return nil, fmt.Errorf("acme: no certificate for %q", hello.ServerName)
}

func (m *LegoAcmeManager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

func (m *LegoAcmeManager) SetDomains(domains []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domains = domains
	zap.S().Infof("acme: tracking %d domain(s): %v", len(domains), domains)
}

func (m *LegoAcmeManager) EnsureCertificates() error {
	m.mu.RLock()
	snapshot := make([]string, len(m.domains))
	copy(snapshot, m.domains)
	m.mu.RUnlock()

	var need []string
	for _, d := range snapshot {
		m.mu.RLock()
		cert := m.certCache[d]
		m.mu.RUnlock()
		if cert == nil || needsRenewal(cert) {
			need = append(need, d)
		}
	}
	if len(need) == 0 {
		zap.S().Debugf("acme: all domains have valid certificates")
		return nil
	}

	// Obtain one certificate per domain to keep things simple and
	// independent – a failure for one domain does not block others.
	var firstErr error
	for _, d := range need {
		if err := m.ObtainCertificate([]string{d}); err != nil {
			zap.S().Errorf("acme: cert for %s failed: %v", d, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (m *LegoAcmeManager) StartRenewalLoop() {
	go func() {
		// Immediate first pass.
		if err := m.EnsureCertificates(); err != nil {
			zap.S().Errorf("acme: initial cert check: %v", err)
		}

		zap.S().Infof("acme: renewal loop started (interval %s)", m.renewalInterval)
		ticker := time.NewTicker(m.renewalInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := m.EnsureCertificates(); err != nil {
					zap.S().Errorf("acme: renewal tick: %v", err)
				}
			case <-m.stopCh:
				zap.S().Info("acme: renewal loop stopped")
				return
			}
		}
	}()
}

func (m *LegoAcmeManager) Stop() {
	select {
	case <-m.stopCh:
		// already closed
	default:
		close(m.stopCh)
	}
}

func needsRenewal(cert *tls.Certificate) bool {
	if cert == nil || len(cert.Certificate) == 0 {
		return true
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return true
	}
	return time.Until(leaf.NotAfter) < 30*24*time.Hour
}

func pemToECKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func ecKeyToPEM(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal ec key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}
