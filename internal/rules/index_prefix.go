package rules

import (
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/mysql"
	"github.com/pingcap/tidb/pkg/parser/types"
)

// defaultBlobPrefixChars is the prefix length applied to indexed TEXT/BLOB
// columns. TiDB, like MySQL, refuses to index these types without an
// explicit prefix length.
const defaultBlobPrefixChars = 255

// IndexPrefixRule adds a prefix length to indexed TEXT/BLOB columns that
// lack one. Oversized explicit prefixes and wide CHAR/VARCHAR keys are
// handled separately by KeyLengthRule.
type IndexPrefixRule struct{}

// Name returns rule name
func (r *IndexPrefixRule) Name() string { return "IndexPrefix" }

// Description returns rule description
func (r *IndexPrefixRule) Description() string {
	return "Add prefix length to indexed TEXT/BLOB columns"
}

// Priority determines rule order
func (r *IndexPrefixRule) Priority() int { return 350 }

// ShouldApply checks if the table has indexed TEXT/BLOB columns without a prefix length
func (r *IndexPrefixRule) ShouldApply(node ast.Node) bool {
	tbl, ok := node.(*ast.CreateTableStmt)
	if !ok {
		return false
	}
	colMap := buildColumnMap(tbl.Cols)
	for _, cons := range tbl.Constraints {
		if !isIndexConstraint(cons.Tp) {
			continue
		}
		for _, key := range cons.Keys {
			if keyNeedsBlobPrefix(key, colMap) {
				return true
			}
		}
	}
	return false
}

// Apply sets prefix lengths for indexed TEXT/BLOB columns
func (r *IndexPrefixRule) Apply(node ast.Node) (ast.Node, error) {
	tbl, ok := node.(*ast.CreateTableStmt)
	if !ok {
		return node, nil
	}
	colMap := buildColumnMap(tbl.Cols)
	for _, cons := range tbl.Constraints {
		if !isIndexConstraint(cons.Tp) {
			continue
		}
		for _, key := range cons.Keys {
			if keyNeedsBlobPrefix(key, colMap) {
				key.Length = defaultBlobPrefixChars
			}
		}
	}
	return tbl, nil
}

// keyNeedsBlobPrefix reports whether an index key part references a
// TEXT/BLOB column without an explicit prefix length.
func keyNeedsBlobPrefix(key *ast.IndexPartSpecification, colMap map[string]*ast.ColumnDef) bool {
	if key.Column == nil || key.Length > 0 {
		return false
	}
	col, ok := colMap[key.Column.Name.L]
	if !ok {
		return false
	}
	return isTextOrBlob(col.Tp)
}

func buildColumnMap(cols []*ast.ColumnDef) map[string]*ast.ColumnDef {
	m := make(map[string]*ast.ColumnDef, len(cols))
	for _, col := range cols {
		m[col.Name.Name.L] = col
	}
	return m
}

func isTextOrBlob(ft *types.FieldType) bool {
	switch ft.GetType() {
	case mysql.TypeTinyBlob, mysql.TypeMediumBlob, mysql.TypeBlob, mysql.TypeLongBlob:
		return true
	default:
		return false
	}
}

func isVarcharOrChar(ft *types.FieldType) bool {
	switch ft.GetType() {
	case mysql.TypeVarchar, mysql.TypeVarString, mysql.TypeString:
		return true
	default:
		return false
	}
}
