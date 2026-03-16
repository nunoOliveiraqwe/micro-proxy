package log

import "go.uber.org/zap/zapcore"

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

const defaultLogLevel = LogLevelInfo

func allLevels() []LogLevel {
	return []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal}
}

func (l LogLevel) String() string {
	return []string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}[l]
}

func (l LogLevel) toZapLevel() zapcore.Level {
	return zapcore.Level(l - 1)
}
