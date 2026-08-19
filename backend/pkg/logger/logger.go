package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"quant-system/backend/internal/config"
)

// Init 初始化 Zap Logger，JSON 格式输出到 stdout（容器内由 Docker 收集日志）
func Init(cfg config.LogConfig) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if cfg.Level != "" {
		if err := level.UnmarshalText([]byte(strings.ToLower(cfg.Level))); err != nil {
			level = zapcore.InfoLevel
		}
	}

	zapCfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Encoding:         "json",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "timestamp",
			LevelKey:       "level",
			MessageKey:     "message",
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeDuration: zapcore.MillisDurationEncoder,
		},
	}

	log, err := zapCfg.Build()
	if err != nil {
		return nil, err
	}
	// 兼容标准库 log（gin 等）
	_ = zap.ReplaceGlobals(log)
	_ = os.Setenv("GIN_MODE", ginMode())
	return log, nil
}

func ginMode() string {
	if mode := os.Getenv("GIN_MODE"); mode != "" {
		return mode
	}
	return "release"
}
