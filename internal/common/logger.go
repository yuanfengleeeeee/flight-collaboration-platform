package common

import (
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger *zap.Logger
	once   sync.Once
)

// InitLogger 初始化全局 logger
// level: debug/info/warn/error; encoding: json/console; output: stdout/file
func InitLogger(level, encoding, output string) {
	once.Do(func() {
		var zapLevel zapcore.Level
		switch level {
		case "debug":
			zapLevel = zapcore.DebugLevel
		case "info":
			zapLevel = zapcore.InfoLevel
		case "warn":
			zapLevel = zapcore.WarnLevel
		case "error":
			zapLevel = zapcore.ErrorLevel
		default:
			zapLevel = zapcore.InfoLevel
		}

		zapCfg := zap.Config{
			Level:            zap.NewAtomicLevelAt(zapLevel),
			Development:      false,
			Encoding:         encoding,
			EncoderConfig:    zap.NewProductionEncoderConfig(),
			OutputPaths:      []string{output},
			ErrorOutputPaths: []string{output},
		}
		// 时间格式用 ISO8601
		zapCfg.EncoderConfig.TimeKey = "time"
		zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

		l, err := zapCfg.Build()
		if err != nil {
			panic("初始化 logger 失败: " + err.Error())
		}
		logger = l
	})
}

// L 返回全局 logger
func L() *zap.Logger {
	if logger == nil {
		// 兜底:未初始化时用默认 logger
		l, _ := zap.NewProduction()
		return l
	}
	return logger
}
