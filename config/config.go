package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	LogConfig LogConfig       `yaml:"log" json:"log"`
	APIServer APIServerConfig `yaml:"api-server" json:"api-server"`
	NetConfig NetworkConfig   `yaml:"net-config" json:"net-config"`
	Session   SessionConfig   `yaml:"session" json:"session"`
}

type SessionConfig struct {
	Lifetime        time.Duration `yaml:"lifetime" json:"lifetime"`
	IdleTimeout     time.Duration `yaml:"idle-timeout" json:"idle-timeout"`
	CleanupInterval time.Duration `yaml:"cleanup-interval" json:"cleanup-interval"`
	CookieDomain    string        `yaml:"cookie-domain" json:"cookie-domain"`
	CookieSecure    bool          `yaml:"cookie-secure" json:"cookie-secure"`
	CookieHttpOnly  bool          `yaml:"cookie-http-only" json:"cookie-http-only"`
	CookieSameSite  string        `yaml:"cookie-same-site" json:"cookie-same-site"`
}

// SameSiteMode converts the string config value to an http.SameSite constant.
func (c SessionConfig) SameSiteMode() http.SameSite {
	switch strings.ToLower(c.CookieSameSite) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	case "lax":
		return http.SameSiteLaxMode
	default:
		return http.SameSiteLaxMode
	}
}

type LogConfig struct {
	LogConfDebug bool   `yaml:"log-debug" json:"log-debug"`
	LogPath      string `yaml:"log-path" json:"log-path"`
	LogLevel     string `yaml:"log-level" json:"log-level"`
	Encoding     string `yaml:"encoding" json:"encoding"`
	ColorEnabled *bool  `yaml:"color" json:"color"`
}

func (c LogConfig) GetEncoding() string {
	if strings.EqualFold(c.Encoding, "json") {
		return "json"
	}
	return "console"
}

func (c LogConfig) IsColorEnabled() bool {
	if c.GetEncoding() == "json" {
		return false
	}
	if c.ColorEnabled == nil {
		return true
	}
	return *c.ColorEnabled
}

type APIServerConfig struct {
	Port             int    `yaml:"port" json:"port"`
	Host             string `yaml:"host" json:"host"`
	AccessLogPath    string `yaml:"access-log-path" json:"access-log-path"`
	IdleTimeoutSecs  int    `yaml:"idle-timeout" json:"idle-timeout"`
	ReadTimeoutSecs  int    `yaml:"read-timeout" json:"read-timeout"`
	WriteTimeoutSecs int    `yaml:"write-timeout" json:"write-timeout"`
}

func DefaultConfiguration() AppConfig {
	return AppConfig{
		LogConfig: LogConfig{
			LogLevel: "INFO",
		},
		APIServer: APIServerConfig{
			Host:             "127.0.0.1",
			Port:             27000,
			IdleTimeoutSecs:  60,
			ReadTimeoutSecs:  60,
			WriteTimeoutSecs: 60,
		},
		Session: SessionConfig{
			Lifetime:        16 * time.Hour,
			IdleTimeout:     60 * time.Minute,
			CleanupInterval: 1 * time.Hour,
			CookieSecure:    false,
			CookieHttpOnly:  true,
			CookieSameSite:  "lax",
		},
	}
}

func LoadConfiguration(path string) (AppConfig, error) {
	conf := DefaultConfiguration()
	if path == "" {
		//no conf to load, we default
		return conf, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return conf, fmt.Errorf("failed to read configuration file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return conf, fmt.Errorf("failed to parse configuration file %q: %w", path, err)
	}
	return conf, nil
}
