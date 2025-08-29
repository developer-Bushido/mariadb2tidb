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
        UNIQUE KEY url (url)
    )`
	stmts, _, err := p.Parse(sql, "", "")
	require.NoError(t, err)
	create := stmts[0].(*ast.CreateTableStmt)

	colURL := create.Cols[0]
	assert.True(t, rule.ShouldApply(colURL))
	newURL, err := rule.Apply(colURL)
	require.NoError(t, err)
	assert.Equal(t, 768, newURL.(*ast.ColumnDef).Tp.GetFlen())

	colInfo := create.Cols[1]
	assert.False(t, rule.ShouldApply(colInfo))
}
