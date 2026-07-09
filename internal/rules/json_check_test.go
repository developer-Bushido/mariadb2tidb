package rules

import (
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONCheckRule_ColumnAndTable(t *testing.T) {
	rule := &JSONCheckRule{}
	p := parser.New()
	sql := `CREATE TABLE t (
        a JSON CHECK (json_valid(a)),
        b LONGTEXT,
        CHECK (json_valid(b))
    )`
	stmts, _, err := p.Parse(sql, "", "")
	require.NoError(t, err)
	create := stmts[0].(*ast.CreateTableStmt)

	// Column-level check
	colA := create.Cols[0]
	assert.True(t, rule.ShouldApply(colA))
	newA, err := rule.Apply(colA)
	require.NoError(t, err)
	assert.Empty(t, newA.(*ast.ColumnDef).Options)

	// Table-level check
	assert.True(t, rule.ShouldApply(create))
	newTable, err := rule.Apply(create)
	require.NoError(t, err)
	assert.Len(t, newTable.(*ast.CreateTableStmt).Constraints, 0)
}
