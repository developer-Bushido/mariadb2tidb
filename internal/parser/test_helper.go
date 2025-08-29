package parser

import (
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
)

// initTestLogger initializes a logger for testing
func initTestLogger() error {
	return utils.InitLogger(true) // development mode for tests
}
