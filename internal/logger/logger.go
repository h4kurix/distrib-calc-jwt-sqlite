package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// sugar всегда инициализирован: до Init работает no-op логгер.
var sugar = zap.NewNop().Sugar()

// Init настраивает логгер. Ошибка возвращается вызывающему, panic не используется.
func Init(env, level string) error {
	var cfg zap.Config
	if env == "production" {
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
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	l, err := cfg.Build()
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}
	sugar = l.Sugar()
	sugar.Infof("logger initialized: env=%s level=%s", env, level)
	return nil
}

func Debug(format string, args ...any) { sugar.Debugf(format, args...) }
func Info(format string, args ...any)  { sugar.Infof(format, args...) }
func Warn(format string, args ...any)  { sugar.Warnf(format, args...) }
func Error(format string, args ...any) { sugar.Errorf(format, args...) }

func Sync() {
	_ = sugar.Sync()
}
