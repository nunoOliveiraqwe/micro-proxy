package config

import (
	"time"

	"github.com/nunoOliveiraqwe/torii/middleware"
)

type IpFlag byte

const (
	Ipv4Flag IpFlag = 1 << iota
	Ipv6Flag IpFlag = Ipv4Flag << 1
	BothFlag IpFlag = Ipv4Flag | Ipv6Flag
)

type NetworkConfig struct {
	HTTPListeners []HTTPListener `yaml:"http" json:"http,omitempty"`
	TCPListeners  []TCPListener  `yaml:"tcp" json:"tcp,omitempty"`
	Global        *GlobalConfig  `yaml:"global" json:"global,omitempty"`
}

type HTTPListener struct {
	Port              int           `yaml:"port" json:"port"`
	Bind              IpFlag        `yaml:"bind" json:"bind"`
	Interface         string        `yaml:"interface" json:"interface,omitempty"`
	TLS               *TLSConfig    `yaml:"tls" json:"tls,omitempty"`
	DisableHTTP2      bool          `yaml:"disable-http2" json:"disable-http2,omitempty"`
	ReadTimeout       time.Duration `yaml:"read-timeout" json:"read-timeout,omitempty"`
	ReadHeaderTimeout time.Duration `yaml:"read-header-timeout" json:"read-header-timeout,omitempty"`
	WriteTimeout      time.Duration `yaml:"write-timeout" json:"write-timeout,omitempty"`
	IdleTimeout       time.Duration `yaml:"idle-timeout" json:"idle-timeout,omitempty"`
	Routes            []Route       `yaml:"routes" json:"routes,omitempty"`
	Default           *RouteTarget  `yaml:"default" json:"default,omitempty"`
}

type GlobalConfig struct {
	Middlewares     []middleware.Config   `yaml:"middlewares" json:"middlewares,omitempty"`
	DisableDefaults bool                  `yaml:"disable-default-middlewares" json:"disable-default-middlewares,omitempty"`
	TrustedProxies  *TrustedProxiesConfig `yaml:"trusted-proxies" json:"trusted-proxies,omitempty"`
}

type TCPListener struct {
	Port            int                 `yaml:"port" json:"port"`
	Bind            string              `yaml:"bind" json:"bind,omitempty"`
	Interface       string              `yaml:"interface" json:"interface,omitempty"`
	Backend         string              `yaml:"backend" json:"backend"`
	Middlewares     []middleware.Config `yaml:"middlewares" json:"middlewares,omitempty"`
	DisableDefaults bool                `yaml:"disable-default-middlewares" json:"disable-default-middlewares,omitempty"`
}

type Route struct {
	Host   string      `yaml:"host" json:"host"`
	Target RouteTarget `yaml:"target" json:"target"`
}

type PathRule struct {
	Pattern         string                `yaml:"pattern" json:"pattern"`
	Backend         *BackendConfig        `yaml:"backend" json:"backend,omitempty"`
	DropQuery       *bool                 `yaml:"drop-query" json:"drop-query,omitempty"`
	StripPrefix     *bool                 `yaml:"strip-prefix" json:"strip-prefix,omitempty"`
	Middlewares     []middleware.Config   `yaml:"middlewares" json:"middlewares,omitempty"`
	DisableDefaults bool                  `yaml:"disable-default-middlewares" json:"disable-default-middlewares,omitempty"`
	TrustedProxies  *TrustedProxiesConfig `yaml:"trusted-proxies" json:"trusted-proxies,omitempty"`
}

type RouteTarget struct {
	Backend         BackendConfig         `yaml:"backend" json:"backend"`
	Middlewares     []middleware.Config   `yaml:"middlewares" json:"middlewares,omitempty"`
	Paths           []PathRule            `yaml:"paths" json:"paths,omitempty"`
	DisableDefaults bool                  `yaml:"disable-default-middlewares" json:"disable-default-middlewares,omitempty"`
	TrustedProxies  *TrustedProxiesConfig `yaml:"trusted-proxies" json:"trusted-proxies,omitempty"`
}

type TrustedProxiesConfig struct {
	Preset          string        `yaml:"preset" json:"preset,omitempty"`
	Ranges          []string      `yaml:"ranges" json:"ranges,omitempty"`
	Header          string        `yaml:"header" json:"header,omitempty"`
	RefreshInterval time.Duration `yaml:"refresh-interval" json:"refresh-interval,omitempty"`
}

type TLSConfig struct {
	UseAcme bool   `yaml:"use-acme" json:"use-acme"`
	Cert    string `yaml:"cert" json:"cert,omitempty"`
	Key     string `yaml:"key" json:"key,omitempty"`
}

type BackendConfig struct {
	Address           string `yaml:"address" json:"address"`
	ReplaceHostHeader bool   `yaml:"replace-host-header" json:"replace-host-header,omitempty"`
}
