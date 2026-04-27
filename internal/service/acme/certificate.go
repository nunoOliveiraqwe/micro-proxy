package acme

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"go.uber.org/zap"
)

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
	name := strings.ToLower(hello.ServerName)

	m.mu.RLock()
	cert, ok := m.certCache[name]
	if !ok {
		if idx := strings.Index(name, "."); idx > 0 {
			cert, ok = m.certCache["*."+name[idx+1:]]
		}
	}
	m.mu.RUnlock()

	if ok {
		return cert, nil
	}
	// return nil so that direct requests to IP are stopped with alert 112
	//	alertUnrecognizedName             alert = 112
	return nil, nil
}

// ResetAll revokes all certificates with the CA, deletes all ACME data
// from the store (certificates, accounts, configuration), and clears the
// in-memory cache. Best-effort: revocation errors are logged but do not
// prevent the store from being wiped.
func (m *LegoAcmeManager) ResetAll() error {
	certs, err := m.store.ListCertificates()
	if err != nil {
		zap.S().Warnf("acme: could not list certificates for revocation: %v", err)
	}

	revoked := 0
	for _, c := range certs {
		if err := m.client.Certificate.Revoke(c.Certificate); err != nil {
			zap.S().Warnf("acme: failed to revoke cert for %s: %v", c.Domain, err)
			continue
		}
		revoked++
		zap.S().Infof("acme: revoked cert for %s", c.Domain)
	}
	if len(certs) > 0 {
		zap.S().Infof("acme: revoked %d/%d certificate(s)", revoked, len(certs))
	}

	if err := m.store.ResetAll(); err != nil {
		return fmt.Errorf("acme: failed to reset store: %w", err)
	}

	m.mu.Lock()
	m.certCache = make(map[string]*tls.Certificate)
	m.mu.Unlock()

	zap.S().Info("acme: all data reset")
	return nil
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
