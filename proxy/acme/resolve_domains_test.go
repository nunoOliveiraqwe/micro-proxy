package acme

import (
	"crypto/tls"
	"testing"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager(domains []string, supplier func() []string) *LegoAcmeManager {
	mgr := &LegoAcmeManager{
		conf: &domain.AcmeConfiguration{
			Domains: domains,
		},
		certCache: make(map[string]*tls.Certificate),
	}
	mgr.domainSupplier = supplier
	return mgr
}

func TestResolveDomains_ExplicitOnly(t *testing.T) {
	mgr := newTestManager([]string{"*.example.com", "example.com"}, nil)
	got := mgr.resolveDomains()
	assert.ElementsMatch(t, []string{"*.example.com", "example.com"}, got)
}

func TestResolveDomains_SupplierOnly(t *testing.T) {
	mgr := newTestManager(nil, func() []string {
		return []string{"app.example.com", "api.example.com"}
	})
	got := mgr.resolveDomains()
	assert.ElementsMatch(t, []string{"app.example.com", "api.example.com"}, got)
}

func TestResolveDomains_ExplicitIgnoresSupplier(t *testing.T) {
	mgr := newTestManager([]string{"*.example.com"}, func() []string {
		return []string{"should-not-appear.example.com"}
	})
	got := mgr.resolveDomains()
	assert.ElementsMatch(t, []string{"*.example.com"}, got)
	assert.NotContains(t, got, "should-not-appear.example.com")
}

func TestResolveDomains_DeduplicatesDuplicates(t *testing.T) {
	mgr := newTestManager(nil, func() []string {
		return []string{"app.example.com", "app.example.com", "api.example.com"}
	})
	got := mgr.resolveDomains()
	assert.ElementsMatch(t, []string{"app.example.com", "api.example.com"}, got)
}

func TestResolveDomains_CaseInsensitive(t *testing.T) {
	mgr := newTestManager(nil, func() []string {
		return []string{"App.Example.COM", "app.example.com"}
	})
	got := mgr.resolveDomains()
	require.Len(t, got, 1)
}

func TestResolveDomains_WildcardSuppressesSingleLevel(t *testing.T) {
	mgr := newTestManager([]string{"*.example.com", "app.example.com", "example.com"}, nil)
	got := mgr.resolveDomains()
	assert.ElementsMatch(t, []string{"*.example.com", "example.com"}, got)
	assert.NotContains(t, got, "app.example.com")
}

func TestResolveDomains_WildcardDoesNotSuppressDeep(t *testing.T) {
	mgr := newTestManager([]string{
		"*.example.com",
		"app.example.com",
		"a.b.c.d.app.example.com",
	}, nil)
	got := mgr.resolveDomains()
	assert.Contains(t, got, "*.example.com")
	assert.Contains(t, got, "a.b.c.d.app.example.com")
	assert.NotContains(t, got, "app.example.com")
}

func TestResolveDomains_NoWildcard(t *testing.T) {
	mgr := newTestManager(nil, func() []string {
		return []string{"app.example.com", "api.example.com", "example.com"}
	})
	got := mgr.resolveDomains()
	assert.ElementsMatch(t, []string{"app.example.com", "api.example.com", "example.com"}, got)
}

func TestResolveDomains_Empty(t *testing.T) {
	mgr := newTestManager(nil, nil)
	got := mgr.resolveDomains()
	assert.Nil(t, got)
}

func TestResolveDomains_SupplierReturnsEmpty(t *testing.T) {
	mgr := newTestManager(nil, func() []string { return nil })
	got := mgr.resolveDomains()
	assert.Nil(t, got)
}
