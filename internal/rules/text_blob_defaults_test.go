package rules

import (
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextBlobDefaultRule(t *testing.T) {
	rule := &TextBlobDefaultRule{}
	p := parser.New()
	sql := `CREATE TABLE t (
        a LONGTEXT NOT NULL DEFAULT '[]',
        b JSON DEFAULT '{}',
        c VARCHAR(100) DEFAULT 'abc',
        d BLOB DEFAULT x'01',
        e TEXT DEFAULT ''
    )`
	stmts, _, err := p.Parse(sql, "", "")
	require.NoError(t, err)
	create := stmts[0].(*ast.CreateTableStmt)

	colA := create.Cols[0]
	assert.True(t, rule.ShouldApply(colA))
	newA, err := rule.Apply(colA)
	require.NoError(t, err)
	for _, opt := range newA.(*ast.ColumnDef).Options {
		assert.NotEqual(t, ast.ColumnOptionDefaultValue, opt.Tp)
	}

	colB := create.Cols[1]
	assert.True(t, rule.ShouldApply(colB))
	newB, err := rule.Apply(colB)
	require.NoError(t, err)
	for _, opt := range newB.(*ast.ColumnDef).Options {
		assert.NotEqual(t, ast.ColumnOptionDefaultValue, opt.Tp)
	}

	// VARCHAR column should remain untouched
	colC := create.Cols[2]
	assert.False(t, rule.ShouldApply(colC))

	colD := create.Cols[3]
	assert.True(t, rule.ShouldApply(colD))
	newD, err := rule.Apply(colD)
	require.NoError(t, err)
	for _, opt := range newD.(*ast.ColumnDef).Options {
		assert.NotEqual(t, ast.ColumnOptionDefaultValue, opt.Tp)
	}

	colE := create.Cols[4]
	assert.True(t, rule.ShouldApply(colE))
	newE, err := rule.Apply(colE)
	require.NoError(t, err)
	for _, opt := range newE.(*ast.ColumnDef).Options {
		assert.NotEqual(t, ast.ColumnOptionDefaultValue, opt.Tp)
	}
}
