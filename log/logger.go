package log

import (
	"strings"

	"github.com/nunoOliveiraqwe/micro-proxy/configuration"
	"github.com/nunoOliveiraqwe/micro-proxy/util"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitLogger(conf configuration.LogConfig) {
	var cfg zap.Config

	if conf.Debug {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()

	}

	internalLogLevel := getLogLevelFromLogStr(conf.LogLevel)
	zapLevel := internalLogLevel.toZapLevel()
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	if conf.LogPath != "" {
		exists := util.FileExists(conf.LogPath)
		if exists {
			cfg.OutputPaths = []string{conf.LogPath, "stdout"}
			cfg.ErrorOutputPaths = []string{conf.LogPath, "stderr"}
		}
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	log, err := cfg.Build()
	if err != nil {
		log = zap.NewNop()
		zap.S().Errorf("Failed to initialize logger: %v", err)
	}
	zap.RedirectStdLog(log)
	zap.ReplaceGlobals(log)
}

func getLogLevelFromLogStr(logLevelStr string) LogLevel {
	all := allLevels()
	for _, level := range all {
		if strings.EqualFold(strings.ToLower(level.String()), strings.ToLower(logLevelStr)) {
			return level
		}
	}
	return defaultLogLevel
}
