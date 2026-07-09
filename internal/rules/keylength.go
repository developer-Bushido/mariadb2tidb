package rules

import (
	"github.com/pingcap/tidb/pkg/parser/ast"
)

// maxKeyLenChars is the longest single-column index key TiDB accepts:
// 3072 bytes with the default max-index-length, which is 768 characters
// in 4-byte utf8mb4 encoding.
// https://docs.pingcap.com/tidb/stable/tidb-limitations
const maxKeyLenChars = 768

// KeyLengthRule caps index key prefix lengths at 768 characters so utf8mb4
// keys stay within TiDB's 3072-byte max-index-length limit. Column
// definitions are left untouched: TiDB supports VARCHAR up to 16383
// characters, only the indexed prefix is limited.
type KeyLengthRule struct{}

// Name returns rule name
func (r *KeyLengthRule) Name() string { return "KeyLength" }

// Description returns description
func (r *KeyLengthRule) Description() string {
	return "Cap index key prefix lengths at 768 characters (3072 bytes in utf8mb4) per TiDB max-index-length"
}

// Priority determines rule order
func (r *KeyLengthRule) Priority() int { return 300 }

// ShouldApply checks if any index key exceeds the TiDB key length limit,
// either through an explicit prefix or via the full width of a CHAR/VARCHAR column.
func (r *KeyLengthRule) ShouldApply(node ast.Node) bool {
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
			if keyNeedsCap(key, colMap) {
				return true
			}
		}
	}
	return false
}

// Apply caps oversized index key prefixes at maxKeyLenChars.
func (r *KeyLengthRule) Apply(node ast.Node) (ast.Node, error) {
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
			if keyNeedsCap(key, colMap) {
				key.Length = maxKeyLenChars
			}
		}
	}
	return tbl, nil
}

// keyNeedsCap reports whether an index key part exceeds maxKeyLenChars.
// Keys without an explicit prefix are measured by the column width for
// CHAR/VARCHAR columns; TEXT/BLOB prefixes are handled by IndexPrefixRule.
func keyNeedsCap(key *ast.IndexPartSpecification, colMap map[string]*ast.ColumnDef) bool {
	if key.Column == nil {
		return false
	}
	if key.Length > maxKeyLenChars {
		return true
	}
	if key.Length > 0 {
		return false
	}
	col, ok := colMap[key.Column.Name.L]
	if !ok {
		return false
	}
	return isVarcharOrChar(col.Tp) && col.Tp.GetFlen() > maxKeyLenChars
}

// isIndexConstraint reports whether a constraint type builds an index key.
func isIndexConstraint(tp ast.ConstraintType) bool {
	switch tp {
	case ast.ConstraintPrimaryKey, ast.ConstraintKey, ast.ConstraintIndex, ast.ConstraintUniq,
		ast.ConstraintUniqKey, ast.ConstraintUniqIndex:
		return true
	default:
		return false
	}
}
