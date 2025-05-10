package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var (
	sugar *zap.SugaredLogger
)

// Init инициализирует zap логгер на основе уровня логирования
func Init(level string) {
	var cfg zap.Config

	if os.Getenv("ENV") == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}

	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.OutputPaths = []string{"stdout"}

	switch level {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	logger, err := cfg.Build()
	if err != nil {
		panic("cannot initialize zap logger: " + err.Error())
	}
	sugar = logger.Sugar()
	sugar.Infof("Logger initialized with level: %s", level)
}

// Debug logs debug messages
func Debug(format string, args ...interface{}) {
	sugar.Debugf(format, args...)
}

// Info logs info messages
func Info(format string, args ...interface{}) {
	sugar.Infof(format, args...)
}

// Error logs error messages
func Error(format string, args ...interface{}) {
	sugar.Errorf(format, args...)
}

// Warn logs warning messages
func Warn(format string, args ...interface{}) {
	sugar.Warnf(format, args...)
}

// Fatal logs a fatal message and exits
func Fatal(format string, args ...interface{}) {
	sugar.Fatalf(format, args...)
}
