package torii

import (
	"flag"

	"go.uber.org/zap"
)

type Flags struct {
	ConfigPath string
	DataDir    string
	Debug      *bool
	LogLevel   *string
	Headless   *bool
}

func RegisterFlags() *Flags {
	f := &Flags{}
	flag.StringVar(&f.ConfigPath, "config", "", "Path to the configuration file")
	flag.StringVar(&f.DataDir, "data-dir", ".", "Directory for database, working config, and runtime state")
	f.Debug = flag.Bool("debug", false, "Enable debug mode")
	f.LogLevel = flag.String("log-level", "", "Log level (overrides config)")
	f.Headless = flag.Bool("headless", false, "Run as a pure proxy with no UI, no API server, and no database")
	return f
}

func (f *Flags) ParseFlags() {
	flag.Parse()
	zap.S().Info("Flags parsed successfully")
}

func (f *Flags) IsHeadless() bool {
	return f.Headless != nil && *f.Headless
}
