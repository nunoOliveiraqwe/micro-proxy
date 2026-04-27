package acme

import (
	"fmt"

	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"go.uber.org/zap"
)

// Bootstrap loads ACME configuration from the database, optionally seeds it
// from the YAML config on first run, and creates a ready-to-use ACME manager.
//
// Returns nil (and no error) when ACME is not configured or disabled.
// The caller must call SetDomainSupplier on the returned manager before
// starting the renewal loop.
func Bootstrap(acmeStore store.AcmeStore, yamlCfg *config.AcmeConfig) (*LegoAcmeManager, error) {
	conf, err := acmeStore.GetConfiguration()
	if err != nil {
		zap.S().Warnf("Failed to read ACME configuration from DB: %v", err)
	}

	if conf == nil && yamlCfg != nil && yamlCfg.Email != "" && yamlCfg.DNSProvider != "" {
		zap.S().Info("No ACME configuration in DB; seeding from YAML config file")
		seeded, seedErr := SeedFromYAML(yamlCfg, acmeStore)
		if seedErr != nil {
			return nil, fmt.Errorf("failed to seed ACME config from YAML: %w", seedErr)
		}
		conf = seeded
	}

	if conf == nil || !conf.Enabled {
		return nil, nil
	}

	mgr, err := NewLegoAcmeManager(conf, acmeStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACME manager: %w", err)
	}

	return mgr, nil
}
