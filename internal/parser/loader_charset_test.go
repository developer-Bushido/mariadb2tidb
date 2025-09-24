package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
)

func TestLoaderWithCharsetMappings(t *testing.T) {
	// Initialize logger for tests
	require.NoError(t, initTestLogger())

	cfg := &config.Config{
		CharsetMappings: map[string]config.CharsetMapping{
			"utf8mb3": {
				TargetCharset:   "utf8mb4",
				TargetCollation: "utf8mb4_0900_ai_ci",
			},
			"utf8": {
				TargetCharset:   "utf8mb4",
				TargetCollation: "utf8mb4_0900_ai_ci",
			},
		},
		CollationMappings: map[string]string{
			"utf8mb3_unicode_ci": "utf8mb4_0900_ai_ci",
			"utf8_unicode_ci":    "utf8mb4_0900_ai_ci",
		},
	}

	loader := NewLoaderWithConfig(cfg)
	writer := NewWriter()

	sql := `CREATE TABLE modelGoals (
		modelId INT(11) unsigned not null,
		requestId VARCHAR(23) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci DEFAULT NULL,
		historyId VARCHAR(23) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci NOT NULL,
		PRIMARY KEY (historyId)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;`

	// Parse the SQL
	stmts, err := loader.LoadFromString(sql)
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	// Write it back
	result, err := writer.WriteStatements(stmts)
	require.NoError(t, err)

	// Check that charset and collation were transformed
	upper := strings.ToUpper(result)
	assert.Contains(t, upper, "CHARACTER SET UTF8MB4", "Charset should be transformed to UTF8MB4")
	assert.Contains(t, upper, "COLLATE UTF8MB4_0900_AI_CI", "Collation should be transformed to utf8mb4_0900_ai_ci")

	// Ensure no problematic combinations remain
	assert.NotContains(t, result, "CHARACTER SET utf8mb3", "Should not contain utf8mb3 charset")
	assert.NotContains(t, result, "CHARACTER SET UTF8 collate utf8mb4_0900_ai_ci", "Should not have incompatible charset/collation combo")
}

func TestLoaderApplyCharsetMappings(t *testing.T) {
	loader := NewLoader()

	// Configure with charset mappings
	charsetMap := map[string]string{
		"utf8mb3": "utf8mb4",
		"utf8":    "utf8mb4",
	}
	collationMap := map[string]string{
		"utf8mb3_unicode_ci": "utf8mb4_0900_ai_ci",
		"utf8_unicode_ci":    "utf8mb4_0900_ai_ci",
	}
	loader.WithCharsetMappings(charsetMap, collationMap)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "utf8mb3 charset transformation",
			input:    "CHARACTER SET utf8mb3",
			expected: "CHARACTER SET utf8mb4",
		},
		{
			name:     "utf8 charset transformation",
			input:    "CHARACTER SET utf8",
			expected: "CHARACTER SET utf8mb4",
		},
		{
			name:     "utf8mb3_unicode_ci collation transformation",
			input:    "COLLATE utf8mb3_unicode_ci",
			expected: "COLLATE utf8mb4_0900_ai_ci",
		},
		{
			name:     "case insensitive charset",
			input:    "character set UTF8MB3",
			expected: "CHARACTER SET utf8mb4",
		},
		{
			name:     "case insensitive collation",
			input:    "collate UTF8MB3_UNICODE_CI",
			expected: "COLLATE utf8mb4_0900_ai_ci",
		},
		{
			name:     "no transformation needed",
			input:    "CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci",
			expected: "CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := loader.applyCharsetMappings(tt.input)
			assert.Contains(t, result, tt.expected, "Expected transformation not found")
		})
	}
}
