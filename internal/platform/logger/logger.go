// Package logger creates process-local structured loggers.
package logger

import (
	"fmt"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(cfg config.LogConfig) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if err := level.Set(cfg.Level); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	settings := zap.NewProductionConfig()
	settings.Level = zap.NewAtomicLevelAt(level)
	settings.Encoding = cfg.Encoding
	settings.OutputPaths = []string{cfg.Output}
	settings.ErrorOutputPaths = []string{cfg.Output}
	settings.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	return settings.Build()
}
