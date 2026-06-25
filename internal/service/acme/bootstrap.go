package acme

import (
	"github.com/nunoOliveiraqwe/torii/config"
	"github.com/nunoOliveiraqwe/torii/internal/store"
	"go.uber.org/zap"
)

// Bootstrap loads ACME configurations from the database (or seeds them from
// the YAML config on first run) and creates one LegoAcmeManager per valid
// configuration.
//
// Returns an empty slice (and no error) when ACME is not configured.
// The caller is responsible for wiring a per-config domain supplier on each
// returned manager before starting its renewal loop.
func Bootstrap(acmeStore store.AcmeStore, yamlCfg []*config.AcmeConfig) ([]*LegoAcmeManager, error) {
	confs, err := acmeStore.GetConfigurations()

	seedFromYaml := false
	if err != nil {
		zap.S().Warnf("Failed to read ACME configurations from DB: %v", err)
		seedFromYaml = true
	} else if len(confs) == 0 {
		zap.S().Info("No ACME configurations found in DB")
		seedFromYaml = true
	}

	if seedFromYaml {
		if len(yamlCfg) == 0 {
			zap.S().Info("No ACME configuration provided via YAML; ACME remains disabled")
			return nil, nil
		}
		return seedFromYAML(acmeStore, yamlCfg)
	}

	managers := make([]*LegoAcmeManager, 0, len(confs))
	for _, conf := range confs {
		if !conf.IsValid() {
			zap.S().Warnf("Skipping invalid ACME configuration id=%d email=%q", conf.ID, conf.Email)
			continue
		}
		mgr, err := NewLegoAcmeManager(conf, acmeStore)
		if err != nil {
			zap.S().Warnf("Failed to bootstrap ACME manager for config id=%d email=%q: %v", conf.ID, conf.Email, err)
			continue
		}
		managers = append(managers, mgr)
	}
	return managers, nil
}

func seedFromYAML(acmeStore store.AcmeStore, yamlCfg []*config.AcmeConfig) ([]*LegoAcmeManager, error) {
	zap.S().Info("Seeding ACME configuration from YAML")
	managers := make([]*LegoAcmeManager, 0, len(yamlCfg))
	for _, conf := range yamlCfg {
		if conf.Email == "" || conf.DNSProvider == "" {
			continue
		}
		zap.S().Debugf("Seeding ACME configuration for email %s and DNS provider %s", conf.Email, conf.DNSProvider)
		acmeConf, err := SeedFromYAML(conf, acmeStore)
		if err != nil {
			zap.S().Errorf("Failed to seed ACME config from YAML: %v", err)
			continue
		}
		if !acmeConf.IsValid() {
			zap.S().Errorf("Invalid ACME configuration from YAML for email %s", conf.Email)
			continue
		}
		legoManager, err := NewLegoAcmeManager(acmeConf, acmeStore)
		if err != nil {
			zap.S().Errorf("Failed to initialize LEGO manager with ACME config from YAML: %v", err)
			continue
		}
		managers = append(managers, legoManager)
	}
	return managers, nil
}
