package rules

import (
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexPrefixRule(t *testing.T) {
	rule := &IndexPrefixRule{}
	p := parser.New()
	sql := `CREATE TABLE t (
        id INT,
        word TEXT,
        name VARCHAR(32),
        data TEXT,
        KEY idx_word (word),
        UNIQUE KEY uniq_name (name),
        KEY explicit (data(10))
    )`
	stmts, _, err := p.Parse(sql, "", "")
	require.NoError(t, err)
	create := stmts[0].(*ast.CreateTableStmt)

	assert.True(t, rule.ShouldApply(create))
	newNode, err := rule.Apply(create)
	require.NoError(t, err)
	newTable := newNode.(*ast.CreateTableStmt)

	// idx_word on TEXT should use 255 prefix
	idxWord := newTable.Constraints[0]
	assert.Equal(t, 255, idxWord.Keys[0].Length)

	// uniq_name on VARCHAR(32) should use length 32
	uniqName := newTable.Constraints[1]
	assert.Equal(t, 32, uniqName.Keys[0].Length)

	// explicit prefix should remain unchanged
	explicit := newTable.Constraints[2]
	assert.Equal(t, 10, explicit.Keys[0].Length)
}
