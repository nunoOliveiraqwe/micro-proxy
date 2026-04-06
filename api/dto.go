package api

// --- FTS DTOs ---

type FtsStatusResponse struct {
	IsFtsCompleted bool `json:"isFtsCompleted"`
}

type CompleteFtsRequest struct {
	Password string `json:"password"`
}

// --- Auth DTOs ---

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type UserIdentityResponse struct {
	Username string `json:"username"`
}

type AcmeConfigResponse struct {
	Email                string `json:"email"`
	DNSProvider          string `json:"dnsProvider"`
	CADirURL             string `json:"caDirUrl"`
	RenewalCheckInterval string `json:"renewalCheckInterval"`
	Enabled              bool   `json:"enabled"`
	Configured           bool   `json:"configured"`
}

type AcmeConfigRequest struct {
	Email                    string                    `json:"email"`
	CADirURL                 string                    `json:"caDirUrl"`
	RenewalCheckInterval     string                    `json:"renewalCheckInterval"`
	Enabled                  bool                      `json:"enabled"`
	DnsProviderConfigRequest *DnsProviderConfigRequest `json:"dns_provider_config_request"`
}

type DnsProviderConfigRequest struct {
	DNSProvider string            `json:"provider"`
	ConfigMap   map[string]string `json:"configurationMap"`
}

type AcmeToggleRequest struct {
	Enabled bool `json:"enabled"`
}

type AcmeCertificateResponse struct {
	Domain    string `json:"domain"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	Active    bool   `json:"active"`
}

type AcmeProviderResponse struct {
	Name   string              `json:"name"`
	Fields []AcmeProviderField `json:"fields"`
}

type AcmeProviderField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"`
	Placeholder string `json:"placeholder"`
}
