package app

import (
	"fmt"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"github.com/nunoOliveiraqwe/torii/proxy"
	"github.com/nunoOliveiraqwe/torii/proxy/acme"
	"go.uber.org/zap"
)

var (
	ErrAcmeAlreadyConfigured = fmt.Errorf("ACME is already configured; reset to reconfigure")
	ErrAcmeNotConfigured     = fmt.Errorf("no ACME configuration exists")
	ErrEmailRequired         = fmt.Errorf("email is required")
	ErrDNSProviderRequired   = fmt.Errorf("DNS provider is required")
	ErrInvalidDNSProvider    = fmt.Errorf("invalid DNS provider")
	ErrInvalidDNSProviderCfg = fmt.Errorf("invalid DNS provider configuration")
	ErrInvalidRenewalFmt     = fmt.Errorf("invalid renewal interval format (use Go duration, e.g. 12h, 6h30m)")
	ErrRenewalTooShort       = fmt.Errorf("renewal interval must be at least 1h")
)

type AcmeConfigResult struct {
	Email                string
	DNSProvider          string
	CADirURL             string
	RenewalCheckInterval string
	Enabled              bool
	Configured           bool
}

// AcmeCertResult is returned by ListCertificates.
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
}

type AcmeService struct {
	store      store.AcmeStore
	reloadAcme func() error
	getProxies func() []*proxy.ProxySnapshot
}

func NewAcmeService(acmeStore store.AcmeStore, reloadAcme func() error, getProxies func() []*proxy.ProxySnapshot) *AcmeService {
	return &AcmeService{
		store:      acmeStore,
		reloadAcme: reloadAcme,
		getProxies: getProxies,
	}
}

func (s *AcmeService) GetConfiguration() (*AcmeConfigResult, error) {
	conf, err := s.store.GetConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ACME configuration: %w", err)
	}
	if conf == nil {
		return &AcmeConfigResult{Configured: false}, nil
	}
	return &AcmeConfigResult{
		Email:                conf.Email,
		DNSProvider:          conf.DNSProvider,
		CADirURL:             conf.CADirURL,
		RenewalCheckInterval: conf.RenewalCheckInterval.String(),
		Enabled:              conf.Enabled,
		Configured:           true,
	}, nil
}

func (s *AcmeService) SaveConfiguration(req *SaveAcmeConfigRequest) error {
	existing, err := s.store.GetConfiguration()
	if err != nil {
		return fmt.Errorf("failed to check existing configuration: %w", err)
	}
	if existing != nil {
		return ErrAcmeAlreadyConfigured
	}

	if req.Email == "" {
		return ErrEmailRequired
	}
	if req.DNSProvider == "" {
		return ErrDNSProviderRequired
	}

	provider, err := acme.GetDNSProvider(req.DNSProvider)
	if err != nil {
		return ErrInvalidDNSProvider
	}

	renewalInterval := 12 * time.Hour
	if req.RenewalCheckInterval != "" {
		parsed, pErr := time.ParseDuration(req.RenewalCheckInterval)
		if pErr != nil {
			return ErrInvalidRenewalFmt
		}
		if parsed < 1*time.Hour {
			return ErrRenewalTooShort
		}
		renewalInterval = parsed
	}

	if err := provider.IsValidMap(req.CredentialMap); err != nil {
		return ErrInvalidDNSProviderCfg
	}
	sf, err := provider.Serialize(req.CredentialMap)
	if err != nil {
		return fmt.Errorf("failed to serialize DNS provider configuration: %w", err)
	}

	conf := &domain.AcmeConfiguration{
		Email:                req.Email,
		DNSProvider:          provider.Name(),
		CADirURL:             req.CADirURL,
		RenewalCheckInterval: renewalInterval,
		Enabled:              req.Enabled,
		SerializedFields:     sf,
	}

	if err := s.store.SaveConfiguration(conf); err != nil {
		return fmt.Errorf("failed to save ACME configuration: %w", err)
	}

	if err := s.reloadAcme(); err != nil {
		return fmt.Errorf("configuration saved but failed to apply: %w", err)
	}

	zap.S().Infow("ACME configuration saved and applied",
		"email", req.Email,
		"dnsProvider", provider.Name(),
		"enabled", req.Enabled,
	)
	return nil
}

func (s *AcmeService) ToggleEnabled(enabled bool) error {
	conf, err := s.store.GetConfiguration()
	if err != nil || conf == nil {
		return ErrAcmeNotConfigured
	}

	conf.Enabled = enabled
	if err := s.store.SaveConfiguration(conf); err != nil {
		return fmt.Errorf("failed to update ACME enabled state: %w", err)
	}

	if err := s.reloadAcme(); err != nil {
		return fmt.Errorf("state saved but failed to apply: %w", err)
	}

	state := "disabled"
	if enabled {
		state = "enabled"
	}
	zap.S().Infof("ACME %s", state)
	return nil
}

func (s *AcmeService) ListCertificates() ([]AcmeCertResult, error) {
	certs, err := s.store.ListCertificates()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ACME certificates: %w", err)
	}

	activeDomains := make(map[string]bool)
	for _, snap := range s.getProxies() {
		if !snap.IsUsingACME {
			continue
		}
		for _, route := range snap.Routes {
			if route.Host != "" {
				activeDomains[route.Host] = true
			}
		}
	}

	results := make([]AcmeCertResult, 0, len(certs))
	for _, c := range certs {
		results = append(results, AcmeCertResult{
			Domain:    c.Domain,
			ExpiresAt: c.ExpiresAt,
			CreatedAt: c.CreatedAt,
			Active:    activeDomains[c.Domain],
		})
	}
	return results, nil
}

func (s *AcmeService) ResetAll() error {
	// Tell system to stop the ACME manager via reload (config gone = nil manager).
	// We nuke first, then reload picks up the empty state.
	if err := s.store.ResetAll(); err != nil {
		return fmt.Errorf("failed to reset ACME data: %w", err)
	}
	if err := s.reloadAcme(); err != nil {
		zap.S().Warnf("ACME data reset but reload failed: %v", err)
	}
	zap.S().Info("ACME data reset successfully")
	return nil
}
