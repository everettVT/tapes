// Package logger provides opinionated logging capabilities for the tapes system
package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(debug bool) *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	// Set log level
	level := zap.InfoLevel
	if debug {
		level = zap.DebugLevel
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		level,
	)

	return zap.New(core, zap.AddCaller())
}
