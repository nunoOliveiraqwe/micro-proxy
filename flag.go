package microProxy

import (
	"flag"

	"go.uber.org/zap"
)

type Flags struct {
	ConfigPath string
	Debug      *bool
	LogLevel   *string
}

func RegisterFlags() *Flags {
	f := &Flags{}
	flag.StringVar(&f.ConfigPath, "config", "", "Path to the configuration file")
	f.Debug = flag.Bool("debug", false, "Enable debug logging (overrides config)")
	f.LogLevel = flag.String("log-level", "", "Log level (overrides config)")
	return f
}

func (f *Flags) ParseFlags() {
	flag.Parse()
	zap.S().Info("Flags parsed successfully")
}
