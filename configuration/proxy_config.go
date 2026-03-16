package configuration

import (
	"time"

	"github.com/nunoOliveiraqwe/micro-proxy/middleware"
)

type IpFlag byte

const (
	Ipv4Flag IpFlag = 1 << iota
	Ipv6Flag IpFlag = 2
	BothFlag IpFlag = Ipv4Flag | Ipv6Flag
)

type ACMEConfig struct {
	Email      string `yaml:"email"`
	Cache      string `yaml:"cache"`
	OpenPort80 bool   `yaml:"open-port-80"`
}

type NetworkConfiguration struct {
	HTTPListeners []HTTPListener `yaml:"http"`
	ACMEConfig    *ACMEConfig    `yaml:"acme"`
	TCPListeners  []TCPListener  `yaml:"tcp"`
}

type HTTPListener struct {
	Port              int           `yaml:"port"`
	Bind              IpFlag        `yaml:"bind"`      // "ipv4" | "ipv6" | "both"
	Interface         string        `yaml:"interface"` // optional
	TLS               *TLSConfig    `yaml:"tls"`       // nil means plain
	ReadTimeout       time.Duration `yaml:"read-timeout"`
	ReadHeaderTimeout time.Duration `yaml:"read-header-timeout"`
	WriteTimeout      time.Duration `yaml:"write-timeout"`
	IdleTimeout       time.Duration `yaml:"idle-timeout"`
	Routes            []Route       `yaml:"routes"`  //for virtual hosting
	Default           *RouteTarget  `yaml:"default"` // catch-all, optional
}

type TCPListener struct {
	Port        int                                  `yaml:"port"`
	Bind        string                               `yaml:"bind"`
	Interface   string                               `yaml:"interface"`
	Backend     string                               `yaml:"backend"`
	Middlewares []middleware.MiddlewareConfiguration `yaml:"middlewares"`
}

type Route struct {
	Host        string                               `yaml:"host"`
	Backend     string                               `yaml:"backend"`
	Middlewares []middleware.MiddlewareConfiguration `yaml:"middlewares"`
}

// RouteTarget is the shared backend+middlewares block
// used by both Route and Listener.Default
type RouteTarget struct {
	Backend     string                               `yaml:"backend"`
	Middlewares []middleware.MiddlewareConfiguration `yaml:"middlewares"`
}

type TLSConfig struct {
	UseAcme bool   `yaml:"use-acme"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}
