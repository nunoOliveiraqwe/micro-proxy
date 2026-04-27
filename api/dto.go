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
	Email                string   `json:"email"`
	DNSProvider          string   `json:"dnsProvider"`
	CADirURL             string   `json:"caDirUrl"`
	RenewalCheckInterval string   `json:"renewalCheckInterval"`
	Enabled              bool     `json:"enabled"`
	Configured           bool     `json:"configured"`
	Domains              []string `json:"domains"`
}

type AcmeConfigRequest struct {
	Email                    string                    `json:"email"`
	CADirURL                 string                    `json:"caDirUrl"`
	RenewalCheckInterval     string                    `json:"renewalCheckInterval"`
	Enabled                  bool                      `json:"enabled"`
	Domains                  []string                  `json:"domains"`
	DnsProviderConfigRequest *DnsProviderConfigRequest `json:"dns_provider_config_request"`
}

type DnsProviderConfigRequest struct {
	DNSProvider string            `json:"provider"`
	ConfigMap   map[string]string `json:"configurationMap"`
}

type AcmeToggleRequest struct {
	Enabled bool `json:"enabled"`
}

type AcmeDomainsRequest struct {
	Domains []string `json:"domains"`
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

type NetworkInterfaceDTO struct {
	Name string `json:"name"`
	IPv4 string `json:"ipv4"`
	IPv6 string `json:"ipv6"`
}

// CreateProxyServerRequest is a thin JSON-friendly wrapper around config.HTTPListener.
// It mirrors the config types exactly but uses string durations ("30s", "1m")
// instead of time.Duration (which JSON encodes as nanoseconds).
// Routes, RouteTarget, PathRule, TLSConfig, and middleware.Config are used directly
// from the config / middleware packages — no extra DTOs needed.
type CreateProxyServerRequest struct {
	Port              int             `json:"port"`
	Bind              int             `json:"bind"` // 1=IPv4, 2=IPv6, 3=both
	Interface         string          `json:"interface,omitempty"`
	TLS               *TLSConfigDTO   `json:"tls,omitempty"`
	DisableHTTP2      bool            `json:"disable_http2,omitempty"`
	ReadTimeout       string          `json:"read_timeout,omitempty"` // Go duration string
	ReadHeaderTimeout string          `json:"read_header_timeout,omitempty"`
	WriteTimeout      string          `json:"write_timeout,omitempty"`
	IdleTimeout       string          `json:"idle_timeout,omitempty"`
	Routes            []RouteDTO      `json:"routes,omitempty"`
	Default           *RouteTargetDTO `json:"default,omitempty"`
}

// TLSConfigDTO mirrors config.TLSConfig with matching JSON tags.
type TLSConfigDTO struct {
	UseAcme bool   `json:"use_acme"`
	Cert    string `json:"cert,omitempty"`
	Key     string `json:"key,omitempty"`
}

// RouteDTO mirrors config.Route with matching JSON tags.
type RouteDTO struct {
	Host   string         `json:"host"`
	Target RouteTargetDTO `json:"target"`
}

// RouteTargetDTO mirrors config.RouteTarget. Middlewares use middleware.Config directly.
type RouteTargetDTO struct {
	Backend         BackendConfigDTO      `json:"backend"`
	Middlewares     []MiddlewareConfigDTO `json:"middlewares,omitempty"`
	Paths           []PathRuleDTO         `json:"paths,omitempty"`
	DisableDefaults bool                  `json:"disable_default_middlewares,omitempty"`
	TrustedProxies  *TrustedProxiesDTO    `json:"trusted_proxies,omitempty"`
}

// PathRuleDTO mirrors config.PathRule.
type PathRuleDTO struct {
	Pattern         string                `json:"pattern"`
	Backend         *BackendConfigDTO     `json:"backend,omitempty"`
	DropQuery       *bool                 `json:"drop_query,omitempty"`
	StripPrefix     *bool                 `json:"strip_prefix,omitempty"`
	Middlewares     []MiddlewareConfigDTO `json:"middlewares,omitempty"`
	DisableDefaults bool                  `json:"disable_default_middlewares,omitempty"`
	TrustedProxies  *TrustedProxiesDTO    `json:"trusted_proxies,omitempty"`
}

// BackendConfigDTO mirrors config.BackendConfig.
type BackendConfigDTO struct {
	Address           string `json:"address"`
	ReplaceHostHeader bool   `json:"replace_host_header,omitempty"`
}

type MiddlewareConfigDTO struct {
	Type    string                 `json:"type"`
	Options map[string]interface{} `json:"options,omitempty"`
}

// TrustedProxiesDTO mirrors config.TrustedProxiesConfig.
type TrustedProxiesDTO struct {
	Preset          string   `json:"preset,omitempty"`
	Ranges          []string `json:"ranges,omitempty"`
	Header          string   `json:"header,omitempty"`
	RefreshInterval string   `json:"refresh_interval,omitempty"`
}
