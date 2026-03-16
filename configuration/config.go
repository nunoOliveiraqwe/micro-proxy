package configuration

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ApplicationConfiguration struct {
	LogConfig LogConfig            `yaml:"log" json:"log"`
	APIServer APIServerConfig      `yaml:"apiServer" json:"apiServer"`
	NetConfig NetworkConfiguration `yaml:"netConfig" json:"netConfig"`
}

type LogConfig struct {
	Debug    bool   `yaml:"debug" json:"debug"`
	LogPath  string `yaml:"logPath" json:"logPath"`
	LogLevel string `yaml:"logLevel" json:"logLevel"`
}

type APIServerConfig struct {
	Port             int    `yaml:"port" json:"port"`
	Host             string `yaml:"host" json:"host"`
	IdleTimeoutSecs  int    `yaml:"idleTimeout" json:"idleTimeout"`
	ReadTimeoutSecs  int    `yaml:"readTimeout" json:"readTimeout"`
	WriteTimeoutSecs int    `yaml:"writeTimeout" json:"writeTimeout"`
}

func DefaultConfiguration() ApplicationConfiguration {
	return ApplicationConfiguration{
		LogConfig: LogConfig{
			Debug:    false,
			LogLevel: "INFO",
		},
		APIServer: APIServerConfig{
			Host:             "127.0.0.1",
			Port:             27000,
			IdleTimeoutSecs:  60,
			ReadTimeoutSecs:  60,
			WriteTimeoutSecs: 60,
		},
	}
}

func LoadConfiguration(path string) (ApplicationConfiguration, error) {
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
