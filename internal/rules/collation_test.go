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

func TestCollationRule_Basic(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())

	// Test rule metadata
	assert.Equal(t, "Collation", rule.Name())
	assert.Equal(t, 100, rule.Priority())
	assert.Contains(t, rule.Description(), "utf8mb4_unicode_*")
	assert.Contains(t, rule.Description(), "latin1_swedish_ci")
}

func TestCollationRule_isTargetCollation(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())

	tests := []struct {
		collation string
		expected  bool
		desc      string
	}{
		{"utf8mb4_unicode_ci", true, "utf8mb4_unicode_ci should match"},
		{"utf8mb4_unicode_520_ci", true, "utf8mb4_unicode_520_ci should match"},
		{"UTF8MB4_UNICODE_CI", true, "case insensitive match"},
		{"latin1_swedish_ci", true, "latin1_swedish_ci should match"},
		{"LATIN1_SWEDISH_CI", true, "case insensitive latin1"},
		{"utf8mb4_0900_ai_ci", false, "already modern collation"},
		{"utf8mb4_general_ci", false, "different utf8mb4 collation"},
		{"utf8_unicode_ci", false, "utf8 (not utf8mb4)"},
		{"latin1_general_ci", false, "different latin1 collation"},
		{"", false, "empty string"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := rule.isTargetCollation(tt.collation)
			assert.Equal(t, tt.expected, result, "collation: %s", tt.collation)
		})
	}
}

func TestCollationRule_ShouldApply(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())
	parser := parser.New()

	tests := []struct {
		sql      string
		expected bool
		desc     string
	}{
		{
			sql: `CREATE TABLE test (
				id int,
				name varchar(100) COLLATE utf8mb4_unicode_ci
			) ENGINE=InnoDB`,
			expected: true,
			desc:     "table with utf8mb4_unicode_ci column",
		},
		{
			sql: `CREATE TABLE test (
				id int,
				name varchar(100) COLLATE latin1_swedish_ci
			) ENGINE=InnoDB`,
			expected: true,
			desc:     "table with latin1_swedish_ci column",
		},
		{
			sql: `CREATE TABLE test (
				id int,
				name varchar(100)
			) ENGINE=InnoDB DEFAULT CHARSET=latin1`,
			expected: true,
			desc:     "table with latin1 charset",
		},
		{
			sql: `CREATE TABLE test (
				id int,
				name varchar(100)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
			expected: true,
			desc:     "table with utf8mb4_unicode_ci table collation",
		},
		{
			sql: `CREATE TABLE test (
				id int,
				name varchar(100) COLLATE utf8mb4_0900_ai_ci
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			expected: false,
			desc:     "table already using modern collation",
		},
		{
			sql: `CREATE TABLE test (
				id int,
				name varchar(100)
			) ENGINE=InnoDB`,
			expected: false,
			desc:     "table with no collation specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			stmts, _, err := parser.Parse(tt.sql, "", "")
			require.NoError(t, err)
			require.Len(t, stmts, 1)

			createTable, ok := stmts[0].(*ast.CreateTableStmt)
			require.True(t, ok)

			result := rule.ShouldApply(createTable)
			assert.Equal(t, tt.expected, result, "SQL: %s", tt.sql)
		})
	}
}

func TestCollationRule_Apply_UTF8MB4Unicode(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())
	parser := parser.New()

	sql := `CREATE TABLE users (
		id int NOT NULL AUTO_INCREMENT,
		username varchar(50) COLLATE utf8mb4_unicode_ci NOT NULL,
		email varchar(255) COLLATE utf8mb4_unicode_520_ci NOT NULL,
		bio text COLLATE utf8mb4_unicode_ci,
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

	stmts, _, err := parser.Parse(sql, "", "")
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	createTable, ok := stmts[0].(*ast.CreateTableStmt)
	require.True(t, ok)

	// Apply the rule
	transformed, err := rule.Apply(createTable)
	require.NoError(t, err)

	transformedTable, ok := transformed.(*ast.CreateTableStmt)
	require.True(t, ok)

	// Check column collations were transformed
	for _, col := range transformedTable.Cols {
		for _, opt := range col.Options {
			if opt.Tp == ast.ColumnOptionCollate {
				assert.Equal(t, "utf8mb4_0900_ai_ci", opt.StrValue,
					"Column %s should have utf8mb4_0900_ai_ci collation", col.Name.Name.O)
			}
		}
	}

	// Check table collation was transformed
	for _, opt := range transformedTable.Options {
		if opt.Tp == ast.TableOptionCollate {
			assert.Equal(t, "utf8mb4_0900_ai_ci", opt.StrValue,
				"Table should have utf8mb4_0900_ai_ci collation")
		}
	}
}

func TestCollationRule_Apply_Latin1Swedish(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())
	parser := parser.New()

	sql := `CREATE TABLE legacy_table (
		id int NOT NULL AUTO_INCREMENT,
		name varchar(100) COLLATE latin1_swedish_ci NOT NULL,
		description text COLLATE latin1_swedish_ci,
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=latin1 COLLATE=latin1_swedish_ci`

	stmts, _, err := parser.Parse(sql, "", "")
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	createTable, ok := stmts[0].(*ast.CreateTableStmt)
	require.True(t, ok)

	// Apply the rule
	transformed, err := rule.Apply(createTable)
	require.NoError(t, err)

	transformedTable, ok := transformed.(*ast.CreateTableStmt)
	require.True(t, ok)

	// Check column collations were transformed
	for _, col := range transformedTable.Cols {
		for _, opt := range col.Options {
			if opt.Tp == ast.ColumnOptionCollate {
				assert.Equal(t, "utf8mb4_0900_ai_ci", opt.StrValue,
					"Column %s should have utf8mb4_0900_ai_ci collation", col.Name.Name.O)
			}
		}
	}

	// Check charset and collation were transformed
	var hasUtf8mb4Charset, hasUtf8mb4Collate bool
	for _, opt := range transformedTable.Options {
		if opt.Tp == ast.TableOptionCharset {
			assert.Equal(t, "utf8mb4", opt.StrValue, "Table should have utf8mb4 charset")
			hasUtf8mb4Charset = true
		}
		if opt.Tp == ast.TableOptionCollate {
			assert.Equal(t, "utf8mb4_0900_ai_ci", opt.StrValue,
				"Table should have utf8mb4_0900_ai_ci collation")
			hasUtf8mb4Collate = true
		}
	}
	assert.True(t, hasUtf8mb4Charset, "Table should have utf8mb4 charset")
	assert.True(t, hasUtf8mb4Collate, "Table should have utf8mb4_0900_ai_ci collation")
}

func TestCollationRule_Apply_Latin1CharsetOnly(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())
	parser := parser.New()

	sql := `CREATE TABLE another_legacy (
		id int NOT NULL AUTO_INCREMENT,
		old_field varchar(50),
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=latin1`

	stmts, _, err := parser.Parse(sql, "", "")
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	createTable, ok := stmts[0].(*ast.CreateTableStmt)
	require.True(t, ok)

	// Apply the rule
	transformed, err := rule.Apply(createTable)
	require.NoError(t, err)

	transformedTable, ok := transformed.(*ast.CreateTableStmt)
	require.True(t, ok)

	// Check charset was transformed and collation was added
	var hasUtf8mb4Charset, hasUtf8mb4Collate bool
	for _, opt := range transformedTable.Options {
		if opt.Tp == ast.TableOptionCharset {
			assert.Equal(t, "utf8mb4", opt.StrValue, "Table should have utf8mb4 charset")
			hasUtf8mb4Charset = true
		}
		if opt.Tp == ast.TableOptionCollate {
			assert.Equal(t, "utf8mb4_0900_ai_ci", opt.StrValue,
				"Table should have utf8mb4_0900_ai_ci collation")
			hasUtf8mb4Collate = true
		}
	}
	assert.True(t, hasUtf8mb4Charset, "Table should have utf8mb4 charset")
	assert.True(t, hasUtf8mb4Collate, "Table should have utf8mb4_0900_ai_ci collation")
}

func TestCollationRule_Apply_NoChangesNeeded(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())
	parser := parser.New()

	sql := `CREATE TABLE already_modern (
		id int NOT NULL AUTO_INCREMENT,
		name varchar(100) COLLATE utf8mb4_0900_ai_ci NOT NULL,
		description text,
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`

	stmts, _, err := parser.Parse(sql, "", "")
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	createTable, ok := stmts[0].(*ast.CreateTableStmt)
	require.True(t, ok)

	// Rule should not apply
	assert.False(t, rule.ShouldApply(createTable))

	// Apply the rule anyway (should not change anything)
	transformed, err := rule.Apply(createTable)
	require.NoError(t, err)

	transformedTable, ok := transformed.(*ast.CreateTableStmt)
	require.True(t, ok)

	// Verify no changes were made
	for _, col := range transformedTable.Cols {
		for _, opt := range col.Options {
			if opt.Tp == ast.ColumnOptionCollate {
				assert.Equal(t, "utf8mb4_0900_ai_ci", opt.StrValue,
					"Already modern collation should remain unchanged")
			}
		}
	}
}

func TestCollationRule_ShouldApply_CreateDatabase(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())
	parser := parser.New()

	tests := []struct {
		sql      string
		expected bool
		desc     string
	}{
		{
			sql:      "CREATE DATABASE test CHARACTER SET = utf8mb4 COLLATE = utf8mb4_unicode_520_ci",
			expected: true,
			desc:     "database with utf8mb4_unicode_520_ci collation",
		},
		{
			sql:      "CREATE DATABASE test CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci",
			expected: false,
			desc:     "database already using modern collation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			stmts, _, err := parser.Parse(tt.sql, "", "")
			require.NoError(t, err)
			require.Len(t, stmts, 1)

			createDB, ok := stmts[0].(*ast.CreateDatabaseStmt)
			require.True(t, ok)

			result := rule.ShouldApply(createDB)
			assert.Equal(t, tt.expected, result, "SQL: %s", tt.sql)
		})
	}
}

func TestCollationRule_Apply_CreateDatabase(t *testing.T) {
	rule := NewCollationRule(config.DefaultConfig())
	parser := parser.New()

	sql := "CREATE DATABASE test CHARACTER SET = latin1 COLLATE = latin1_swedish_ci"

	stmts, _, err := parser.Parse(sql, "", "")
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	createDB, ok := stmts[0].(*ast.CreateDatabaseStmt)
	require.True(t, ok)

	transformed, err := rule.Apply(createDB)
	require.NoError(t, err)

	transformedDB, ok := transformed.(*ast.CreateDatabaseStmt)
	require.True(t, ok)

	var charset, collate string
	for _, opt := range transformedDB.Options {
		switch opt.Tp {
		case ast.DatabaseOptionCharset:
			charset = opt.Value
		case ast.DatabaseOptionCollate:
			collate = opt.Value
		}
	}

	assert.Equal(t, "utf8mb4", charset, "Database should have utf8mb4 charset")
	assert.Equal(t, "utf8mb4_0900_ai_ci", collate, "Database should have utf8mb4_0900_ai_ci collation")
}
