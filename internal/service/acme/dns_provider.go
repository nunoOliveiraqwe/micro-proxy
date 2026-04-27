package acme

import (
	"encoding/json"
	"fmt"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
)

type DNSProviderFactory interface {
	Name() string
	Fields() []ProviderField
	IsValidMap(configurationMap map[string]string) error
	Serialize(configurationMap map[string]string) ([]byte, error)
	Create(serializedBlob []byte) (challenge.Provider, error)
}

// ProviderField -> used to inject values to the UI, e.g API-KEY, Username+Password combo, but for now, i'll only support cloudflare
type ProviderField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"`
	Placeholder string `json:"placeholder"`
}

type cloudflareProviderFactory struct{}

func (*cloudflareProviderFactory) Name() string { return "cloudflare" }

func (*cloudflareProviderFactory) IsValidMap(configurationMap map[string]string) error {
	if _, ok := configurationMap["api_token"]; !ok {
		return fmt.Errorf("cloudflare: api_token is required")
	}
	return nil
}

func (c *cloudflareProviderFactory) Serialize(confMap map[string]string) ([]byte, error) {
	err := c.IsValidMap(confMap)
	if err != nil {
		return nil, err
	}
	return json.Marshal(confMap)
}

func (*cloudflareProviderFactory) Fields() []ProviderField {
	return []ProviderField{
		{Key: "api_token", Label: "API Token", Required: true, Sensitive: true,
			Placeholder: "Cloudflare API token with Zone:Read and DNS:Edit permissions"},
	}
}

func (c *cloudflareProviderFactory) Create(serializedBlob []byte) (challenge.Provider, error) {
	var creds map[string]string
	err := json.Unmarshal(serializedBlob, &creds)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: failed to unmarshal credentials: %w", err)
	}
	err = c.IsValidMap(creds)
	if err != nil {
		return nil, err
	}
	token := creds["api_token"]
	if token == "" {
		return nil, fmt.Errorf("cloudflare: api_token is required")
	}
	cfg := cloudflare.NewDefaultConfig()
	cfg.AuthToken = token
	cfg.ZoneToken = token
	return cloudflare.NewDNSProviderConfig(cfg)
}

var registry map[string]DNSProviderFactory

func init() {
	registry = make(map[string]DNSProviderFactory)
	// Register built-in providers here
	cf := cloudflareProviderFactory{}
	registry[cf.Name()] = &cf
}

func GetDNSProvider(name string) (DNSProviderFactory, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unsupported DNS provider: %q", name)
	}
	return f, nil
}

func ListDNSProviders() []string {
	providers := make([]string, 0)
	for k, _ := range registry {
		providers = append(providers, k)
	}
	return providers
}
