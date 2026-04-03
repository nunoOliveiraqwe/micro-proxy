package config

import (
	"time"

	"github.com/nunoOliveiraqwe/torii/middleware"
)

type IpFlag byte

const (
	Ipv4Flag IpFlag = 1 << iota
	Ipv6Flag IpFlag = 2
	BothFlag IpFlag = Ipv4Flag | Ipv6Flag
)

type NetworkConfig struct {
	HTTPListeners []HTTPListener `yaml:"http"`
	TCPListeners  []TCPListener  `yaml:"tcp"`
	Global        *GlobalConfig  `yaml:"global"`
}

type HTTPListener struct {
	Port              int           `yaml:"port"`
	Bind              IpFlag        `yaml:"bind"`
	Interface         string        `yaml:"interface"`
	TLS               *TLSConfig    `yaml:"tls"`
	ReadTimeout       time.Duration `yaml:"read-timeout"`
	ReadHeaderTimeout time.Duration `yaml:"read-header-timeout"`
	WriteTimeout      time.Duration `yaml:"write-timeout"`
	IdleTimeout       time.Duration `yaml:"idle-timeout"`
	Routes            []Route       `yaml:"routes"`
	Default           *RouteTarget  `yaml:"default"`
}

type GlobalConfig struct {
	Middlewares []middleware.Config `yaml:"middlewares" json:"middlewares"`
}

type TCPListener struct {
	Port        int                 `yaml:"port"`
	Bind        string              `yaml:"bind"`
	Interface   string              `yaml:"interface"`
	Backend     string              `yaml:"backend"`
	Middlewares []middleware.Config `yaml:"middlewares"`
}

type Route struct {
	Host   string      `yaml:"host"`
	Target RouteTarget `yaml:"target"`
}

type PathRule struct {
	Pattern     string              `yaml:"pattern"`
	Middlewares []middleware.Config `yaml:"middlewares"`
}

type RouteTarget struct {
	Backend     string              `yaml:"backend"`
	Middlewares []middleware.Config `yaml:"middlewares"`
	Paths       []PathRule          `yaml:"paths"`
}

type TLSConfig struct {
	UseAcme bool   `yaml:"use-acme"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}
