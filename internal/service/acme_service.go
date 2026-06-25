package service

import (
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/service/acme"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"go.uber.org/zap"
)

var (
	ErrAcmeAlreadyConfigured     = fmt.Errorf("ACME is already configured; reset to reconfigure")
	ErrAcmeNotConfigured         = fmt.Errorf("no ACME configuration exists")
	ErrLegoManagerNotInitialized = fmt.Errorf("ACME manager is not initialized")
	ErrEmailRequired             = fmt.Errorf("email is required")
	ErrDNSProviderRequired       = fmt.Errorf("DNS provider is required")
	ErrInvalidDNSProvider        = fmt.Errorf("invalid DNS provider")
	ErrInvalidDNSProviderCfg     = fmt.Errorf("invalid DNS provider configuration")
	ErrInvalidRenewalFmt         = fmt.Errorf("invalid renewal interval format (use Go duration, e.g. 12h, 6h30m)")
	ErrRenewalTooShort           = fmt.Errorf("renewal interval must be at least 1h")
	ErrDomainOwnedByOtherConfig  = fmt.Errorf("domain is already claimed by another ACME configuration")
	ErrMultipleAutoDiscover      = fmt.Errorf("another ACME configuration already has auto-discover enabled; only one is allowed")
)

type AcmeConfigResult struct {
	Email                string
	DNSProvider          string
	CADirURL             string
	RenewalCheckInterval string
	Enabled              bool
	Configured           bool
	DNSResolvers         []string
}

type AcmeCertResult struct {
	Domain    string
	ExpiresAt time.Time
	CreatedAt time.Time
	Active    bool
}

type SaveAcmeConfigRequest struct {
	Email                string
	CADirURL             string
	RenewalCheckInterval string
	Enabled              bool
	DNSProvider          string
	CredentialMap        map[string]string
	Domains              []string
	DNSResolvers         []string
	AutoDiscover         bool
}

type AcmeRegisteredProxy struct {
	DomainSupplier func() []string
}

type acmeEntry struct {
	configID     int
	domains      []string
	autoDiscover bool
	manager      *acme.LegoAcmeManager
}

type acmeSnapshot struct {
	entries       []*acmeEntry
	index         *domainIndex
	autoFillEntry *acmeEntry
}

type AcmeService struct {
	store store.AcmeStore

	registeredProxyMu sync.RWMutex
	registeredProxy   []*AcmeRegisteredProxy

	current atomic.Pointer[acmeSnapshot]

	mu sync.Mutex // guards SaveConfiguration / reload / Add / Delete
}

func NewAcmeService(acmeStore store.AcmeStore, conf []*config.AcmeConfig) *AcmeService {
	svc := &AcmeService{store: acmeStore}

	managers, err := acme.Bootstrap(acmeStore, conf)
	if err != nil {
		zap.S().Errorf("Failed to initialize ACME managers: %v. HTTPS will not work properly if ACME is required", err)
	}

	snap, err := svc.buildSnapshotFromManagers(managers)
	if err != nil {
		zap.S().Errorf("Failed to build ACME snapshot: %v", err)
		snap = emptySnapshot()
	}
	svc.current.Store(snap)
	svc.wireDomainSuppliers(snap)
	return svc
}

func (s *AcmeService) RegisterProxy(p *AcmeRegisteredProxy) {
	s.registeredProxyMu.Lock()
	s.registeredProxy = append(s.registeredProxy, p)
	s.registeredProxyMu.Unlock()
}

func (s *AcmeService) NotifyDomainsChanged() {
	snap := s.current.Load()
	if snap == nil {
		return
	}
	for _, e := range snap.entries {
		if !e.manager.IsStarted() {
			continue
		}
		mgr := e.manager
		go func() {
			if err := mgr.EnsureCertificates(); err != nil {
				zap.S().Errorf("acme: ensure certs after domain change: %v", err)
			}
		}()
	}
}

func (s *AcmeService) GetConfiguration() (*domain.AcmeConfiguration, error) {
	confs, err := s.store.GetConfigurations()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ACME configurations: %w", err)
	}
	if len(confs) == 0 {
		return nil, nil
	}
	return confs[0], nil
}

func (s *AcmeService) ListConfigurations() ([]*domain.AcmeConfiguration, error) {
	confs, err := s.store.GetConfigurations()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ACME configurations: %w", err)
	}
	return confs, nil
}

func (s *AcmeService) SaveConfiguration(req *SaveAcmeConfigRequest) error {
	existing, err := s.store.GetConfigurations()
	if err != nil {
		return fmt.Errorf("failed to check existing configuration: %w", err)
	}
	if len(existing) > 0 {
		return ErrAcmeAlreadyConfigured
	}
	_, err = s.AddConfiguration(req)
	return err
}

func (s *AcmeService) AddConfiguration(req *SaveAcmeConfigRequest) (*domain.AcmeConfiguration, error) {
	conf, err := s.buildConfigurationFromRequest(req)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.assertNoDomainConflict(conf, 0); err != nil {
		return nil, err
	}
	if err := s.assertSingleAutoDiscover(conf, 0); err != nil {
		return nil, err
	}

	if err := s.store.SaveConfiguration(conf); err != nil {
		return nil, fmt.Errorf("failed to save ACME configuration: %w", err)
	}

	if err := s.reloadLocked(); err != nil {
		return nil, fmt.Errorf("configuration saved but failed to bring manager online: %w", err)
	}

	zap.S().Infow("ACME configuration saved and applied",
		"id", conf.ID,
		"email", conf.Email,
		"dnsProvider", conf.DNSProvider,
		"enabled", conf.Enabled,
	)
	return conf, nil
}

func (s *AcmeService) DeleteConfiguration(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.store.DeleteConfiguration(id); err != nil {
		return fmt.Errorf("failed to delete ACME configuration: %w", err)
	}
	if err := s.reloadLocked(); err != nil {
		return fmt.Errorf("configuration deleted but failed to refresh managers: %w", err)
	}
	return nil
}

func (s *AcmeService) ToggleEnabled(enabled bool) error {
	confs, err := s.store.GetConfigurations()
	if err != nil {
		return fmt.Errorf("failed to read ACME configuration: %w", err)
	}
	if len(confs) == 0 {
		return ErrAcmeNotConfigured
	}
	return s.ToggleConfigEnabled(confs[0].ID, enabled)
}

func (s *AcmeService) ToggleConfigEnabled(id int, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conf, err := s.store.GetConfiguration(id)
	if err != nil {
		return fmt.Errorf("failed to read ACME configuration: %w", err)
	}
	if conf == nil {
		return ErrAcmeNotConfigured
	}
	if conf.Enabled == enabled {
		return nil
	}
	conf.Enabled = enabled
	if err := s.store.SaveConfiguration(conf); err != nil {
		return fmt.Errorf("failed to update ACME enabled state: %w", err)
	}
	if err := s.reloadLocked(); err != nil {
		return fmt.Errorf("enabled-state updated but failed to refresh managers: %w", err)
	}
	zap.S().Infof("ACME configuration id=%d %s", id, stateLabel(enabled))
	return nil
}

func (s *AcmeService) UpdateDomains(domains []string) error {
	confs, err := s.store.GetConfigurations()
	if err != nil {
		return fmt.Errorf("failed to read ACME configuration: %w", err)
	}
	if len(confs) == 0 {
		return ErrAcmeNotConfigured
	}
	return s.UpdateConfigDomains(confs[0].ID, domains)
}

func (s *AcmeService) UpdateConfigDomains(id int, domains []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conf, err := s.store.GetConfiguration(id)
	if err != nil {
		return fmt.Errorf("failed to read ACME configuration: %w", err)
	}
	if conf == nil {
		return ErrAcmeNotConfigured
	}

	conf.Domains = domains
	if err := s.assertNoDomainConflict(conf, id); err != nil {
		return err
	}
	if err := s.store.SaveConfiguration(conf); err != nil {
		return fmt.Errorf("failed to update ACME domains: %w", err)
	}
	if err := s.reloadLocked(); err != nil {
		return fmt.Errorf("domains updated but failed to refresh managers: %w", err)
	}
	s.NotifyDomainsChanged()
	zap.S().Infow("ACME domains updated", "id", id, "domains", domains)
	return nil
}

func (s *AcmeService) ListCertificates() ([]AcmeCertResult, error) {
	certs, err := s.store.ListCertificates()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ACME certificates: %w", err)
	}

	activeDomains := make(map[string]bool)
	for _, d := range s.collectAllDomains() {
		activeDomains[strings.ToLower(d)] = true
	}

	results := make([]AcmeCertResult, 0, len(certs))
	for _, c := range certs {
		results = append(results, AcmeCertResult{
			Domain:    c.Domain,
			ExpiresAt: c.ExpiresAt,
			CreatedAt: c.CreatedAt,
			Active:    isCertDomainActive(strings.ToLower(c.Domain), activeDomains),
		})
	}
	return results, nil
}

func (s *AcmeService) ResetAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := s.current.Load()
	if snap != nil {
		zap.S().Info("Resetting ACME data: stopping renewal loops and revoking certificates")
		for _, e := range snap.entries {
			e.manager.Stop()
			if err := e.manager.ResetAll(); err != nil {
				zap.S().Warnf("ACME manager id=%d reset had errors (continuing): %v", e.configID, err)
			}
		}
	}

	if err := s.store.ResetAll(); err != nil {
		return fmt.Errorf("failed to reset ACME data: %w", err)
	}
	s.current.Store(emptySnapshot())
	zap.S().Info("ACME data reset successfully")
	return nil
}

func (s *AcmeService) GetAcmeTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: s.getCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

func (s *AcmeService) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	snap := s.current.Load()
	if snap == nil || len(snap.entries) == 0 {
		return nil, fmt.Errorf("ACME is not configured")
	}
	name := strings.ToLower(hello.ServerName)
	entry := snap.lookup(name)
	if entry == nil {
		// no manager owns this name; return nil so TLS responds with
		// unrecognized_name (alert 112), matching the previous behavior.
		return nil, nil
	}
	return entry.manager.GetCertificate(hello)
}

func (s *AcmeService) Start() {
	snap := s.current.Load()
	if snap == nil {
		return
	}
	for _, e := range snap.entries {
		e.manager.Start()
	}
}

func (s *AcmeService) Stop() {
	snap := s.current.Load()
	if snap == nil {
		return
	}
	for _, e := range snap.entries {
		e.manager.Stop()
	}
}

func (s *AcmeService) buildConfigurationFromRequest(req *SaveAcmeConfigRequest) (*domain.AcmeConfiguration, error) {
	if req.Email == "" {
		return nil, ErrEmailRequired
	}
	if req.DNSProvider == "" {
		return nil, ErrDNSProviderRequired
	}
	provider, err := acme.GetDNSProvider(req.DNSProvider)
	if err != nil {
		return nil, ErrInvalidDNSProvider
	}
	renewalInterval := 12 * time.Hour
	if req.RenewalCheckInterval != "" {
		parsed, pErr := time.ParseDuration(req.RenewalCheckInterval)
		if pErr != nil {
			return nil, ErrInvalidRenewalFmt
		}
		if parsed < 1*time.Hour {
			return nil, ErrRenewalTooShort
		}
		renewalInterval = parsed
	}
	if err := provider.IsValidMap(req.CredentialMap); err != nil {
		return nil, ErrInvalidDNSProviderCfg
	}
	sf, err := provider.Serialize(req.CredentialMap)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize DNS provider configuration: %w", err)
	}
	return &domain.AcmeConfiguration{
		Email:                req.Email,
		DNSProvider:          provider.Name(),
		CADirURL:             req.CADirURL,
		RenewalCheckInterval: renewalInterval,
		Enabled:              req.Enabled,
		SerializedFields:     sf,
		Domains:              req.Domains,
		DNSResolvers:         acme.NormalizeDNSResolvers(req.DNSResolvers),
		AutoDiscover:         req.AutoDiscover,
	}, nil
}

func (s *AcmeService) assertSingleAutoDiscover(candidate *domain.AcmeConfiguration, excludeID int) error {
	if !candidate.AutoDiscover {
		return nil
	}
	confs, err := s.store.GetConfigurations()
	if err != nil {
		return fmt.Errorf("failed to read ACME configurations: %w", err)
	}
	for _, c := range confs {
		if c.ID == excludeID { //relevant for UPDATE, where the candidate is already in the store
			continue
		}
		if c.AutoDiscover {
			return fmt.Errorf("%w: config id=%d (%s) already has it enabled",
				ErrMultipleAutoDiscover, c.ID, c.Email)
		}
	}
	return nil
}

func (s *AcmeService) assertNoDomainConflict(candidate *domain.AcmeConfiguration, excludeID int) error {
	if len(candidate.Domains) == 0 {
		return nil
	}
	confs, err := s.store.GetConfigurations()
	if err != nil {
		return fmt.Errorf("failed to read ACME configurations: %w", err)
	}
	claimed := make(map[string]int)
	for _, c := range confs {
		if c.ID == excludeID { //relevant for UPDATE, where the candidate is already in the store
			continue
		}
		for _, d := range c.Domains {
			claimed[strings.ToLower(d)] = c.ID
		}
	}
	for _, d := range candidate.Domains {
		if owner, ok := claimed[strings.ToLower(d)]; ok {
			return fmt.Errorf("%w: %q is owned by config id=%d", ErrDomainOwnedByOtherConfig, d, owner)
		}
	}
	return nil
}

func (s *AcmeService) reloadLocked() error {
	// Stop the previous set first so cancelled renewal loops don't race the
	// new ones during the brief window between manager creation and swap.
	oldSnap := s.current.Load()
	if oldSnap != nil {
		for _, e := range oldSnap.entries {
			e.manager.Stop()
		}
	}

	confs, err := s.store.GetConfigurations()
	if err != nil {
		return fmt.Errorf("failed to read ACME configurations: %w", err)
	}
	managers := make([]*acme.LegoAcmeManager, 0, len(confs))
	for _, conf := range confs {
		if !conf.IsValid() {
			zap.S().Warnf("Skipping invalid ACME configuration id=%d email=%q", conf.ID, conf.Email)
			continue
		}
		mgr, err := acme.NewLegoAcmeManager(conf, s.store)
		if err != nil {
			zap.S().Errorf("Failed to create ACME manager for config id=%d: %v", conf.ID, err)
			continue
		}
		managers = append(managers, mgr)
	}

	snap, err := s.buildSnapshotFromManagers(managers)
	if err != nil {
		return err
	}
	s.current.Store(snap)
	s.wireDomainSuppliers(snap)

	for _, e := range snap.entries {
		conf, err := s.store.GetConfiguration(e.configID)
		if err == nil && conf != nil && conf.Enabled {
			e.manager.Start()
		}
	}
	return nil
}

func (s *AcmeService) buildSnapshotFromManagers(managers []*acme.LegoAcmeManager) (*acmeSnapshot, error) {
	if len(managers) == 0 {
		return emptySnapshot(), nil
	}
	confs, err := s.store.GetConfigurations()
	if err != nil {
		return nil, fmt.Errorf("failed to read ACME configurations: %w", err)
	}
	confByID := make(map[int]*domain.AcmeConfiguration, len(confs))
	for _, c := range confs {
		confByID[c.ID] = c
	}

	entries := make([]*acmeEntry, 0, len(managers))
	for _, mgr := range managers {
		conf, ok := confByID[mgr.ConfigID()]
		if !ok {
			zap.S().Warnf("acme: manager references missing config id=%d; dropping", mgr.ConfigID())
			continue
		}
		entries = append(entries, &acmeEntry{
			configID:     conf.ID,
			domains:      normalizeDomains(conf.Domains),
			autoDiscover: conf.AutoDiscover,
			manager:      mgr,
		})
	}

	snap := &acmeSnapshot{
		entries: entries,
		index:   newDomainIndex(),
	}
	autoDiscoverCandidates := 0
	for _, e := range entries {
		if e.autoDiscover {
			autoDiscoverCandidates++
			snap.autoFillEntry = e
		}
		for _, d := range e.domains {
			if prev := snap.index.insert(d, e); prev != nil && prev != e {
				zap.S().Warnf("acme: domain %q claimed by both config id=%d and id=%d; keeping latest",
					d, prev.configID, e.configID)
			}
		}
	}
	if autoDiscoverCandidates > 1 {
		zap.S().Warnf("acme: %d configurations have auto-discover enabled; this is invalid and the auto-fill target is ambiguous — disabling proxy fallback",
			autoDiscoverCandidates)
		snap.autoFillEntry = nil
		// Clear the per-entry flag so wireDomainSuppliers doesn't double-feed.
		for _, e := range entries {
			e.autoDiscover = false
		}
	}
	return snap, nil
}

func (s *AcmeService) wireDomainSuppliers(snap *acmeSnapshot) {
	for _, e := range snap.entries {
		entry := e
		entry.manager.SetDomainSupplier(func() []string {
			var out []string
			if len(entry.domains) > 0 {
				out = append(out, entry.domains...)
			}
			if entry.autoDiscover {
				out = append(out, s.collectProxyDomains()...)
			}
			return out
		})
	}
}

func (s *AcmeService) collectProxyDomains() []string {
	s.registeredProxyMu.RLock()
	defer s.registeredProxyMu.RUnlock()
	var out []string
	for _, p := range s.registeredProxy {
		if p.DomainSupplier != nil {
			out = append(out, p.DomainSupplier()...)
		}
	}
	return out
}

func (s *AcmeService) collectAllDomains() []string {
	snap := s.current.Load()
	if snap == nil {
		return nil
	}
	var out []string
	autoDiscoverActive := false
	for _, e := range snap.entries {
		if len(e.domains) > 0 {
			out = append(out, e.domains...)
		}
		if e.autoDiscover {
			autoDiscoverActive = true
		}
	}
	if autoDiscoverActive {
		out = append(out, s.collectProxyDomains()...)
	}
	return out
}

func (snap *acmeSnapshot) lookup(name string) *acmeEntry {
	return snap.index.lookup(name)
}

func emptySnapshot() *acmeSnapshot {
	return &acmeSnapshot{index: newDomainIndex()}
}

func normalizeDomains(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, d := range in {
		dl := strings.ToLower(strings.TrimSpace(d))
		if dl == "" {
			continue
		}
		if _, ok := seen[dl]; ok {
			continue
		}
		seen[dl] = struct{}{}
		out = append(out, dl)
	}
	return out
}

func stateLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func isCertDomainActive(certDomain string, activeDomains map[string]bool) bool {
	if activeDomains[certDomain] {
		return true
	}
	if strings.HasPrefix(certDomain, "*.") {
		parent := certDomain[2:]
		for host := range activeDomains {
			if idx := strings.Index(host, "."); idx > 0 && host[idx+1:] == parent {
				return true
			}
		}
	} else {
		if idx := strings.Index(certDomain, "."); idx > 0 {
			wildcard := "*." + certDomain[idx+1:]
			if activeDomains[wildcard] {
				return true
			}
		}
	}
	return false
}
