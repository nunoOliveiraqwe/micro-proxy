package acme

import (
	"fmt"
	"time"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/resolve"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"go.uber.org/zap"
)

func SeedFromYAML(yamlCfg *config.AcmeConfig, acmeStore store.AcmeStore) (*domain.AcmeConfiguration, error) {
	provider, err := GetDNSProvider(yamlCfg.DNSProvider)
	if err != nil {
		return nil, fmt.Errorf("invalid DNS provider %q: %w", yamlCfg.DNSProvider, err)
	}

	resolved := make(map[string]string, len(yamlCfg.Credentials))
	for k, v := range yamlCfg.Credentials {
		val, err := resolve.ResolveValue(v)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve credential %q: %w", k, err)
		}
		resolved[k] = val
	}

	if err := provider.IsValidMap(resolved); err != nil {
		return nil, fmt.Errorf("invalid DNS provider credentials: %w", err)
	}
	sf, err := provider.Serialize(resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize DNS provider config: %w", err)
	}

	renewalInterval := 12 * time.Hour
	if yamlCfg.RenewalCheckInterval != "" {
		parsed, pErr := time.ParseDuration(yamlCfg.RenewalCheckInterval)
		if pErr != nil {
			return nil, fmt.Errorf("invalid renewal-check-interval %q: %w", yamlCfg.RenewalCheckInterval, pErr)
		}
		if parsed < 1*time.Hour {
			return nil, fmt.Errorf("renewal-check-interval must be at least 1h, got %s", parsed)
		}
		renewalInterval = parsed
	}

	conf := &domain.AcmeConfiguration{
		Email:                yamlCfg.Email,
		DNSProvider:          provider.Name(),
		CADirURL:             yamlCfg.CADirURL,
		RenewalCheckInterval: renewalInterval,
		Enabled:              yamlCfg.Enabled,
		SerializedFields:     sf,
		Domains:              yamlCfg.Domains,
	}

	if err := acmeStore.SaveConfiguration(conf); err != nil {
		return nil, fmt.Errorf("failed to save seeded ACME config: %w", err)
	}
	zap.S().Infow("ACME configuration seeded from YAML",
		"email", conf.Email,
		"dnsProvider", conf.DNSProvider,
		"enabled", conf.Enabled,
	)
	return conf, nil
}
