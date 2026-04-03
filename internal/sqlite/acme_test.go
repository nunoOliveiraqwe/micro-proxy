package sqlite_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/domain"
	"github.com/nunoOliveiraqwe/torii/internal/sqlite"
	"github.com/nunoOliveiraqwe/torii/proxy/acme"
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
	}

	if err := store.SaveConfiguration(conf); err != nil {
		t.Fatalf("SaveConfiguration: %v", err)
	}

	loaded, err := store.GetConfiguration()
	if err != nil {
		t.Fatalf("GetConfiguration: %v", err)
	}
	if loaded == nil {
		t.Fatal("GetConfiguration returned nil")
	}

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

func TestAcmeStore_GetConfiguration_ReturnsNilWhenEmpty(t *testing.T) {
	db := openTestDB(t)
	store := sqlite.NewAcmeStore(db)

	conf, err := store.GetConfiguration()
	if err != nil {
		t.Fatalf("GetConfiguration: %v", err)
	}
	if conf != nil {
		t.Fatalf("expected nil, got %+v", conf)
	}
}

func TestAcmeStore_SaveConfiguration_Upsert(t *testing.T) {
	db := openTestDB(t)
	store := sqlite.NewAcmeStore(db)

	factory, _ := acme.GetDNSProvider("cloudflare")
	blob1, _ := factory.Serialize(map[string]string{"api_token": "first-token"})
	blob2, _ := factory.Serialize(map[string]string{"api_token": "second-token"})

	err := store.SaveConfiguration(&domain.AcmeConfiguration{
		Email:                "a@b.com",
		DNSProvider:          "cloudflare",
		SerializedFields:     blob1,
		RenewalCheckInterval: 12 * time.Hour,
		Enabled:              true,
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	err = store.SaveConfiguration(&domain.AcmeConfiguration{
		Email:                "x@y.com",
		DNSProvider:          "cloudflare",
		SerializedFields:     blob2,
		RenewalCheckInterval: 1 * time.Hour,
		Enabled:              false,
	})
	if err != nil {
		t.Fatalf("upsert save: %v", err)
	}

	loaded, err := store.GetConfiguration()
	if err != nil {
		t.Fatalf("get after upsert: %v", err)
	}
	if loaded.Email != "x@y.com" {
		t.Errorf("Email = %q, want %q", loaded.Email, "x@y.com")
	}
	if loaded.Enabled {
		t.Error("Enabled should be false after upsert")
	}

	var m map[string]string
	_ = json.Unmarshal(loaded.SerializedFields, &m)
	if m["api_token"] != "second-token" {
		t.Errorf("api_token = %q, want %q", m["api_token"], "second-token")
	}
}
