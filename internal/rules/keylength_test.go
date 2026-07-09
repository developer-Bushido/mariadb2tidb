package rules

import (
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyLengthRule(t *testing.T) {
	rule := &KeyLengthRule{}
	p := parser.New()
	sql := `CREATE TABLE t (
        url VARCHAR(1023) NOT NULL,
        info VARCHAR(100) NOT NULL,
        body TEXT,
        UNIQUE KEY url (url),
        KEY idx_info (info),
        KEY idx_body (body(900))
    )`
	stmts, _, err := p.Parse(sql, "", "")
	require.NoError(t, err)
	create := stmts[0].(*ast.CreateTableStmt)

	assert.True(t, rule.ShouldApply(create))
	newNode, err := rule.Apply(create)
	require.NoError(t, err)
	newTable := newNode.(*ast.CreateTableStmt)

	// Column definitions must never be truncated
	assert.Equal(t, 1023, newTable.Cols[0].Tp.GetFlen())

	// Index over VARCHAR(1023) gets a 768-char prefix
	assert.Equal(t, 768, newTable.Constraints[0].Keys[0].Length)

	// Index over VARCHAR(100) stays untouched
	assert.Less(t, newTable.Constraints[1].Keys[0].Length, 1)

	// Explicit oversized prefix is capped at 768
	assert.Equal(t, 768, newTable.Constraints[2].Keys[0].Length)

	// Rule is idempotent: applying again changes nothing
	assert.False(t, rule.ShouldApply(newTable))
}

func TestKeyLengthRuleSkipsCompliantTable(t *testing.T) {
	rule := &KeyLengthRule{}
	p := parser.New()
	sql := `CREATE TABLE t (
        name VARCHAR(255) NOT NULL,
        PRIMARY KEY (name)
    )`
	stmts, _, err := p.Parse(sql, "", "")
	require.NoError(t, err)

	assert.False(t, rule.ShouldApply(stmts[0].(*ast.CreateTableStmt)))
}
