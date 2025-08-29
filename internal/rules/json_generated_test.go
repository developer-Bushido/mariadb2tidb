package rules

import (
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonGeneratedRule_ConvertColumn(t *testing.T) {
	rule := &JsonGeneratedRule{}
	p := parser.New()
	sql := `CREATE TABLE t (
        id INT,
        posCol INT GENERATED ALWAYS AS (json_value(pos,'$.c')) VIRTUAL,
        pos JSON,
        KEY idx_posCol (posCol)
    )`
	stmts, _, err := p.Parse(sql, "", "")
	require.NoError(t, err)
	create := stmts[0].(*ast.CreateTableStmt)

	assert.True(t, rule.ShouldApply(create))
	newNode, err := rule.Apply(create)
	require.NoError(t, err)

	newCreate := newNode.(*ast.CreateTableStmt)
	require.Len(t, newCreate.Cols, 3)
	assert.Equal(t, "id", newCreate.Cols[0].Name.Name.O)
	assert.Equal(t, "posCol", newCreate.Cols[1].Name.Name.O)
	assert.Equal(t, "pos", newCreate.Cols[2].Name.Name.O)

	// posCol should have no generated options after transformation
	for _, opt := range newCreate.Cols[1].Options {
		assert.NotEqual(t, ast.ColumnOptionGenerated, opt.Tp)
	}

	require.Len(t, newCreate.Constraints, 1)
	assert.Equal(t, "idx_posCol", newCreate.Constraints[0].Name)
}
