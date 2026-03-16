package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"
)

type MicroProxyAcmeManager struct {
	acmeManager    *autocert.Manager
	allowedDomains map[string]bool
	mutex          sync.Mutex
}

func newMicroProxyAcmeManager(allowedDomains []string, email, cacheDir string) *MicroProxyAcmeManager {
	zap.S().Infof("Initializing ACME manager with email %s and cache directory %s", email, cacheDir)
	acme := &MicroProxyAcmeManager{
		allowedDomains: make(map[string]bool),
		mutex:          sync.Mutex{},
	}
	acme.addDomain(allowedDomains)
	acme.acmeManager = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      email,
		HostPolicy: acme.dynamicHostPolicy,
		Cache:      autocert.DirCache(cacheDir),
	}
	return acme
}

func (m *MicroProxyAcmeManager) addDomain(domains []string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, domain := range domains {
		zap.S().Debugf("Adding domain %s to ACME allowed domains", domain)
		m.allowedDomains[domain] = true
	}
}

func (acme *MicroProxyAcmeManager) dynamicHostPolicy(_ context.Context, host string) error {
	acme.mutex.Lock()
	defer acme.mutex.Unlock()
	if _, ok := acme.allowedDomains[host]; ok {
		return nil
	}
	return fmt.Errorf("acme/autocert: host %q not configured in allowedDomains", host)
}

func (acme *MicroProxyAcmeManager) bindAcmeHandlerToPort80(handler http.Handler) http.Handler {
	zap.S().Infof("Binding ACME HTTP handler to port 80 server (hopefully)")
	return acme.acmeManager.HTTPHandler(handler)
}

func (acme *MicroProxyAcmeManager) getTlsConfig() *tls.Config {
	return acme.acmeManager.TLSConfig()
}
