package service

import (
	"testing"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/stretchr/testify/assert"
)

type fakeAcmeStore struct {
	confs []*domain.AcmeConfiguration
}

func (f *fakeAcmeStore) GetConfigurations() ([]*domain.AcmeConfiguration, error) {
	return f.confs, nil
}

func (f *fakeAcmeStore) GetConfiguration(id int) (*domain.AcmeConfiguration, error) {
	for _, c := range f.confs {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}

func (f *fakeAcmeStore) SaveConfiguration(conf *domain.AcmeConfiguration) error {
	if conf.ID == 0 {
		conf.ID = len(f.confs) + 1
		f.confs = append(f.confs, conf)
		return nil
	}
	for i, c := range f.confs {
		if c.ID == conf.ID {
			f.confs[i] = conf
			return nil
		}
	}
	f.confs = append(f.confs, conf)
	return nil
}

func (f *fakeAcmeStore) DeleteConfiguration(id int) error {
	out := f.confs[:0]
	for _, c := range f.confs {
		if c.ID != id {
			out = append(out, c)
		}
	}
	f.confs = out
	return nil
}

func (f *fakeAcmeStore) GetAccountFor(string) (*domain.AcmeAccount, error) { return nil, nil }
func (f *fakeAcmeStore) SaveAccount(*domain.AcmeAccount) error             { return nil }
func (f *fakeAcmeStore) GetCertificate(string) (*domain.AcmeCertificate, error) {
	return nil, nil
}
func (f *fakeAcmeStore) SaveCertificate(*domain.AcmeCertificate) error { return nil }
func (f *fakeAcmeStore) ListCertificates() ([]*domain.AcmeCertificate, error) {
	return nil, nil
}
func (f *fakeAcmeStore) ResetAll() error {
	f.confs = nil
	return nil
}

func newSnapshotFromEntries(entries ...*acmeEntry) *acmeSnapshot {
	snap := &acmeSnapshot{
		entries: entries,
		index:   newDomainIndex(),
	}
	autoCount := 0
	for _, e := range entries {
		for _, d := range e.domains {
			snap.index.insert(d, e)
		}
		if e.autoDiscover {
			autoCount++
			snap.autoFillEntry = e
		}
	}
	if autoCount > 1 {
		snap.autoFillEntry = nil
		for _, e := range entries {
			e.autoDiscover = false
		}
	}
	return snap
}

func TestSnapshotLookup_ExactAndWildcard(t *testing.T) {
	cf := &acmeEntry{configID: 1, domains: []string{"*.example.com"}}
	r53 := &acmeEntry{configID: 2, domains: []string{"bar.org", "www.bar.org"}}
	snap := newSnapshotFromEntries(cf, r53)

	assert.Same(t, cf, snap.lookup("api.example.com"), "wildcard *.example.com should match api.example.com")
	assert.Same(t, cf, snap.lookup("anything.example.com"))
	assert.Same(t, r53, snap.lookup("bar.org"))
	assert.Same(t, r53, snap.lookup("www.bar.org"))
	assert.Nil(t, snap.lookup("unknown.test"), "unrouted SNI should return nil")
}

func TestSnapshotLookup_ExactBeatsWildcard(t *testing.T) {
	wildcard := &acmeEntry{configID: 1, domains: []string{"*.foo.com"}}
	exact := &acmeEntry{configID: 2, domains: []string{"api.foo.com"}}
	snap := newSnapshotFromEntries(wildcard, exact)

	assert.Same(t, exact, snap.lookup("api.foo.com"), "exact domain match should win over wildcard")
	assert.Same(t, wildcard, snap.lookup("other.foo.com"))
}

func TestAssertNoDomainConflict_RejectsCrossConfigClaim(t *testing.T) {
	svc := &AcmeService{
		store: &fakeAcmeStore{
			confs: []*domain.AcmeConfiguration{
				{ID: 1, Domains: []string{"app.foo.com"}},
			},
		},
	}
	candidate := &domain.AcmeConfiguration{Domains: []string{"app.foo.com"}}
	err := svc.assertNoDomainConflict(candidate, 0)
	assert.ErrorIs(t, err, ErrDomainOwnedByOtherConfig)
}

func TestAssertNoDomainConflict_AllowsSameConfigUpdate(t *testing.T) {
	svc := &AcmeService{
		store: &fakeAcmeStore{
			confs: []*domain.AcmeConfiguration{
				{ID: 1, Domains: []string{"app.foo.com"}},
			},
		},
	}
	candidate := &domain.AcmeConfiguration{ID: 1, Domains: []string{"app.foo.com", "api.foo.com"}}
	assert.NoError(t, svc.assertNoDomainConflict(candidate, 1))
}

func TestCollectProxyDomains_UsedOnlyByAutoFillEntry(t *testing.T) {
	svc := &AcmeService{}
	svc.RegisterProxy(&AcmeRegisteredProxy{
		DomainSupplier: func() []string { return []string{"discovered.example.com"} },
	})

	withDomains := &acmeEntry{configID: 1, domains: []string{"*.foo.com"}}
	autoFill := &acmeEntry{configID: 2, autoDiscover: true}
	snap := newSnapshotFromEntries(withDomains, autoFill)
	svc.current.Store(snap)

	// The auto-discover entry has no declared domains; the proxy feeds it.
	all := svc.collectAllDomains()
	assert.Contains(t, all, "*.foo.com")
	assert.Contains(t, all, "discovered.example.com")
}

func TestCollectAllDomains_NoAutoFillWhenMultipleAutoDiscoverConfigs(t *testing.T) {
	svc := &AcmeService{}
	svc.RegisterProxy(&AcmeRegisteredProxy{
		DomainSupplier: func() []string { return []string{"discovered.example.com"} },
	})

	// Two configs with AutoDiscover=true → ambiguous → snapshot builder
	// clears both flags and disables the proxy fallback.
	a := &acmeEntry{configID: 1, autoDiscover: true}
	b := &acmeEntry{configID: 2, autoDiscover: true}
	snap := newSnapshotFromEntries(a, b)
	svc.current.Store(snap)

	all := svc.collectAllDomains()
	assert.NotContains(t, all, "discovered.example.com")
}

func TestAssertSingleAutoDiscover_RejectsSecondAutoDiscoverConfig(t *testing.T) {
	svc := &AcmeService{
		store: &fakeAcmeStore{
			confs: []*domain.AcmeConfiguration{
				{ID: 1, Email: "a@x.com", AutoDiscover: true},
			},
		},
	}
	candidate := &domain.AcmeConfiguration{Email: "b@x.com", AutoDiscover: true}
	err := svc.assertSingleAutoDiscover(candidate, 0)
	assert.ErrorIs(t, err, ErrMultipleAutoDiscover)
}

func TestAssertSingleAutoDiscover_AllowsTogglingOwnRow(t *testing.T) {
	svc := &AcmeService{
		store: &fakeAcmeStore{
			confs: []*domain.AcmeConfiguration{
				{ID: 1, Email: "a@x.com", AutoDiscover: true},
			},
		},
	}
	candidate := &domain.AcmeConfiguration{ID: 1, Email: "a@x.com", AutoDiscover: true}
	assert.NoError(t, svc.assertSingleAutoDiscover(candidate, 1))
}

func TestAssertSingleAutoDiscover_PassthroughWhenNotSet(t *testing.T) {
	svc := &AcmeService{
		store: &fakeAcmeStore{
			confs: []*domain.AcmeConfiguration{
				{ID: 1, AutoDiscover: true},
			},
		},
	}
	candidate := &domain.AcmeConfiguration{AutoDiscover: false}
	assert.NoError(t, svc.assertSingleAutoDiscover(candidate, 0))
}
