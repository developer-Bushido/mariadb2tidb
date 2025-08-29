package utils

import (
	"go.uber.org/zap"
)

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
