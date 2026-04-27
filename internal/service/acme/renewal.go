package acme

import (
	"time"

	"go.uber.org/zap"
)

func (m *LegoAcmeManager) EnsureCertificates() error {
	domains := m.resolveDomains()

	var need []string
	for _, d := range domains {
		m.mu.RLock()
		cert := m.certCache[d]
		m.mu.RUnlock()
		if cert == nil || needsRenewal(cert) {
			need = append(need, d)
		}
	}
	if len(need) == 0 {
		zap.S().Debugf("acme: all %d domain(s) have valid certificates", len(domains))
		return nil
	}

	// Group domains that can share a single SAN certificate to conserve
	// Let's Encrypt rate limits (50 certs / registered domain / week).
	//
	// Wildcard domains (*.example.com) are always issued individually
	// because mixing a wildcard with concrete names in one request adds
	// complexity and the wildcard already covers its sub-domains.
	//
	// Non-wildcard domains are grouped by their registered parent
	// (everything after the first dot) so that e.g. app.example.com,
	// api.example.com, and example.com become one SAN request.
	batches := groupDomainBatches(need)

	var firstErr error
	for _, batch := range batches {
		if err := m.ObtainCertificate(batch); err != nil {
			zap.S().Errorf("acme: cert for %v failed: %v", batch, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (m *LegoAcmeManager) StartRenewalLoop() {
	go func() {
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
