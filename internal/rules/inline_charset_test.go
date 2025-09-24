package rules

import (
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
)

func TestInlineCharsetRule_Basic(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CharsetMappings = map[string]config.CharsetMapping{
		"utf8mb3": {
			TargetCharset:   "utf8mb4",
			TargetCollation: "utf8mb4_0900_ai_ci",
		},
		"utf8": {
			TargetCharset:   "utf8mb4",
			TargetCollation: "utf8mb4_0900_ai_ci",
		},
	}
	cfg.CollationMappings = map[string]string{
		"utf8mb3_unicode_ci": "utf8mb4_0900_ai_ci",
		"utf8_unicode_ci":    "utf8mb4_0900_ai_ci",
	}

	rule := NewInlineCharsetRule(cfg)

	// Test rule metadata
	assert.Equal(t, "InlineCharset", rule.Name())
	assert.Equal(t, 50, rule.Priority())
	assert.Contains(t, rule.Description(), "inline")
}

func TestInlineCharsetRule_ShouldApply(t *testing.T) {
	rule := NewInlineCharsetRule(config.DefaultConfig())
	parser := parser.New()

	tests := []struct {
		sql      string
		expected bool
		desc     string
	}{
		{
			sql: `CREATE TABLE test (
				id INT,
				name VARCHAR(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci
			)`,
			expected: true,
			desc:     "column with inline charset and collation",
		},
		{
			sql: `CREATE TABLE test (
				id INT,
				name VARCHAR(100) CHARACTER SET utf8
			)`,
			expected: true,
			desc:     "column with inline charset only",
		},
		{
			sql: `CREATE TABLE test (
				id INT,
				name VARCHAR(100) COLLATE utf8_unicode_ci
			)`,
			expected: true,
			desc:     "column with inline collation only",
		},
		{
			sql: `CREATE TABLE test (
				id INT,
				name VARCHAR(100)
			)`,
			expected: false,
			desc:     "column without inline charset/collation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			stmts, _, err := parser.Parse(tt.sql, "", "")
			require.NoError(t, err)
			require.Len(t, stmts, 1)

			createTable, ok := stmts[0].(*ast.CreateTableStmt)
			require.True(t, ok)
			require.Len(t, createTable.Cols, 2)

			// Check the second column (name column)
			nameCol := createTable.Cols[1]
			result := rule.ShouldApply(nameCol)
			assert.Equal(t, tt.expected, result, "SQL: %s", tt.sql)
		})
	}
}

func TestInlineCharsetRule_Apply(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CharsetMappings = map[string]config.CharsetMapping{
		"utf8mb3": {
			TargetCharset:   "utf8mb4",
			TargetCollation: "utf8mb4_0900_ai_ci",
		},
	}
	cfg.CollationMappings = map[string]string{
		"utf8mb3_unicode_ci": "utf8mb4_0900_ai_ci",
	}

	rule := NewInlineCharsetRule(cfg)
	parser := parser.New()

	sql := `CREATE TABLE test (
		id INT,
		name VARCHAR(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci
	)`

	stmts, _, err := parser.Parse(sql, "", "")
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	createTable, ok := stmts[0].(*ast.CreateTableStmt)
	require.True(t, ok)
	require.Len(t, createTable.Cols, 2)

	nameCol := createTable.Cols[1]
	require.True(t, rule.ShouldApply(nameCol))

	// Apply the rule
	transformed, err := rule.Apply(nameCol)
	require.NoError(t, err)

	transformedCol, ok := transformed.(*ast.ColumnDef)
	require.True(t, ok)

	// Check that charset and collation were transformed
	if transformedCol.Tp != nil {
		charset := transformedCol.Tp.GetCharset()
		collation := transformedCol.Tp.GetCollate()

		if charset != "" {
			assert.Equal(t, "utf8mb4", charset, "Charset should be transformed")
		}
		if collation != "" {
			assert.Equal(t, "utf8mb4_0900_ai_ci", collation, "Collation should be transformed")
		}
	}
}
