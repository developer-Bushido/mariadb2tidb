// Package utils holds small shared helpers, currently the process-wide
// zap logger.
package utils

import (
	"go.uber.org/zap"
)

// Logger is the process-wide logger; use GetLogger instead of reading it directly.
var Logger *zap.Logger

// InitLogger initializes the global logger
func InitLogger(development bool) error {
	var err error
	if development {
		Logger, err = zap.NewDevelopment()
	} else {
		Logger, err = zap.NewProduction()
	}
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
