// Package utils holds small shared helpers, currently the process-wide
// zap logger.
package utils

import (
	"go.uber.org/zap"
)

// Logger is the process-wide logger; use GetLogger instead of reading it directly.
var Logger *zap.Logger

// InitLogger initializes the global logger: debug-level development output
// when development is set, otherwise warnings only, as plain console lines.
// Errors are returned to the CLI, which reports them itself.
func InitLogger(development bool) error {
	var err error
	if development {
		Logger, err = zap.NewDevelopment()
		return err
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	cfg.Encoding = "console"
	cfg.DisableCaller = true
	cfg.DisableStacktrace = true
	cfg.EncoderConfig.TimeKey = ""
	Logger, err = cfg.Build()
	return err
}

// GetLogger returns the global logger instance
func GetLogger() *zap.Logger {
	if Logger == nil {
		// Fallback to no-op logger if not initialized
		Logger = zap.NewNop()
	}
	return Logger
}
