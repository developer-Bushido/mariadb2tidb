package rules

import (
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionDefaultRule(t *testing.T) {
	rule := &FunctionDefaultRule{}
	p := parser.New()
	sql := `CREATE TABLE t (
        a VARCHAR(100) DEFAULT replace(uuid(), '-', ''),
        b VARCHAR(100) DEFAULT uuid(),
        c VARCHAR(100) DEFAULT 'plain',
        d DATETIME DEFAULT current_timestamp(),
        e TINYINT DEFAULT dayofweek(d)
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

	colC := create.Cols[2]
	assert.False(t, rule.ShouldApply(colC))

	colD := create.Cols[3]
	assert.False(t, rule.ShouldApply(colD))

	colE := create.Cols[4]
	assert.True(t, rule.ShouldApply(colE))
	newE, err := rule.Apply(colE)
	require.NoError(t, err)
	for _, opt := range newE.(*ast.ColumnDef).Options {
		assert.NotEqual(t, ast.ColumnOptionDefaultValue, opt.Tp)
	}
}
