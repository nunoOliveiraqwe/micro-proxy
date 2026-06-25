package sqlite_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/service/acme"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
)

func openTestDB(t *testing.T) *sqlite.DB {
	t.Helper()
	db := sqlite.NewDB(":memory:")
	if err := db.Open(); err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAcmeStore_SaveAndGetConfiguration_CloudflareProvider(t *testing.T) {
	db := openTestDB(t)
	store := sqlite.NewAcmeStore(db)

	factory, err := acme.GetDNSProvider("cloudflare")
	if err != nil {
		t.Fatalf("get cloudflare factory: %v", err)
	}

	configMap := map[string]string{"api_token": "test-token-abc123"}

	if err := factory.IsValidMap(configMap); err != nil {
		t.Fatalf("IsValidMap: %v", err)
	}

	serialized, err := factory.Serialize(configMap)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	conf := &domain.AcmeConfiguration{
		Email:                "admin@example.com",
		DNSProvider:          factory.Name(),
		SerializedFields:     serialized,
		CADirURL:             "https://acme-staging-v02.api.letsencrypt.org/directory",
		RenewalCheckInterval: 6 * time.Hour,
		Enabled:              true,
		DNSResolvers:         []string{"1.1.1.1:53", "8.8.8.8:53"},
	}

	if err := store.SaveConfiguration(conf); err != nil {
		t.Fatalf("SaveConfiguration: %v", err)
	}
	if conf.ID == 0 {
		t.Fatal("SaveConfiguration did not assign an ID")
	}

	all, err := store.GetConfigurations()
	if err != nil {
		t.Fatalf("GetConfigurations: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("GetConfigurations len = %d, want 1", len(all))
	}
	loaded := all[0]

	if loaded.Email != conf.Email {
		t.Errorf("Email = %q, want %q", loaded.Email, conf.Email)
	}
	if loaded.DNSProvider != "cloudflare" {
		t.Errorf("DNSProvider = %q, want %q", loaded.DNSProvider, "cloudflare")
	}
	if loaded.CADirURL != conf.CADirURL {
		t.Errorf("CADirURL = %q, want %q", loaded.CADirURL, conf.CADirURL)
	}
	if loaded.RenewalCheckInterval != 6*time.Hour {
		t.Errorf("RenewalCheckInterval = %v, want %v", loaded.RenewalCheckInterval, 6*time.Hour)
	}
	if !loaded.Enabled {
		t.Error("Enabled = false, want true")
	}
	if !reflect.DeepEqual(loaded.DNSResolvers, conf.DNSResolvers) {
		t.Errorf("DNSResolvers = %v, want %v", loaded.DNSResolvers, conf.DNSResolvers)
	}

	if loaded.SerializedFields == nil {
		t.Fatal("SerializedFields is nil after load")
	}

	var loadedMap map[string]string
	if err := json.Unmarshal(loaded.SerializedFields, &loadedMap); err != nil {
		t.Fatalf("unmarshal loaded SerializedFields: %v", err)
	}
	if loadedMap["api_token"] != "test-token-abc123" {
		t.Errorf("api_token = %q, want %q", loadedMap["api_token"], "test-token-abc123")
	}

	_, err = factory.Create(loaded.SerializedFields)
	if err != nil {
		t.Fatalf("factory.Create from DB blob: %v", err)
	}
}

func TestAcmeStore_GetConfigurations_EmptyWhenNoRows(t *testing.T) {
	db := openTestDB(t)
	store := sqlite.NewAcmeStore(db)

	confs, err := store.GetConfigurations()
	if err != nil {
		t.Fatalf("GetConfigurations: %v", err)
	}
	if len(confs) != 0 {
		t.Fatalf("expected empty, got %+v", confs)
	}
}

func TestAcmeStore_SaveConfiguration_InsertVsUpdate(t *testing.T) {
	db := openTestDB(t)
	store := sqlite.NewAcmeStore(db)

	factory, _ := acme.GetDNSProvider("cloudflare")
	blob1, _ := factory.Serialize(map[string]string{"api_token": "first-token"})
	blob2, _ := factory.Serialize(map[string]string{"api_token": "second-token"})

	first := &domain.AcmeConfiguration{
		Email:                "a@b.com",
		DNSProvider:          "cloudflare",
		SerializedFields:     blob1,
		RenewalCheckInterval: 12 * time.Hour,
		Enabled:              true,
		DNSResolvers:         []string{"1.1.1.1:53"},
	}
	if err := store.SaveConfiguration(first); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Second save with no ID inserts another row, not overwrite.
	second := &domain.AcmeConfiguration{
		Email:                "x@y.com",
		DNSProvider:          "cloudflare",
		SerializedFields:     blob2,
		RenewalCheckInterval: 1 * time.Hour,
		Enabled:              false,
		DNSResolvers:         []string{"9.9.9.9:53", "8.8.4.4:53"},
	}
	if err := store.SaveConfiguration(second); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("second insert reused ID %d", first.ID)
	}

	all, err := store.GetConfigurations()
	if err != nil {
		t.Fatalf("GetConfigurations: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 configurations, got %d", len(all))
	}

	// Update by ID changes only the targeted row.
	second.Enabled = true
	second.DNSResolvers = []string{"1.0.0.1:53"}
	if err := store.SaveConfiguration(second); err != nil {
		t.Fatalf("update by id: %v", err)
	}

	got, err := store.GetConfiguration(second.ID)
	if err != nil {
		t.Fatalf("GetConfiguration: %v", err)
	}
	if got == nil {
		t.Fatalf("GetConfiguration(%d) returned nil", second.ID)
	}
	if !got.Enabled {
		t.Error("Enabled should be true after update")
	}
	if !reflect.DeepEqual(got.DNSResolvers, []string{"1.0.0.1:53"}) {
		t.Errorf("DNSResolvers = %v, want %v", got.DNSResolvers, []string{"1.0.0.1:53"})
	}

	var m map[string]string
	_ = json.Unmarshal(got.SerializedFields, &m)
	if m["api_token"] != "second-token" {
		t.Errorf("api_token = %q, want %q", m["api_token"], "second-token")
	}
}

func TestAcmeStore_DeleteConfiguration(t *testing.T) {
	db := openTestDB(t)
	store := sqlite.NewAcmeStore(db)

	factory, _ := acme.GetDNSProvider("cloudflare")
	blob, _ := factory.Serialize(map[string]string{"api_token": "tok"})

	conf := &domain.AcmeConfiguration{
		Email:                "a@b.com",
		DNSProvider:          "cloudflare",
		SerializedFields:     blob,
		RenewalCheckInterval: 12 * time.Hour,
		Enabled:              true,
	}
	if err := store.SaveConfiguration(conf); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := store.DeleteConfiguration(conf.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	all, err := store.GetConfigurations()
	if err != nil {
		t.Fatalf("GetConfigurations: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 configurations after delete, got %d", len(all))
	}
}
