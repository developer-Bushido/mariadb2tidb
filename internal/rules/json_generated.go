package rules

import (
	"strings"

	"github.com/pingcap/tidb/pkg/parser/ast"
)

// JsonGeneratedRule converts generated columns using JSON functions into
// regular columns. TiDB does not allow expressions with JSON functions in
// generated columns. Reference: Step 12 in legacy universal_tidb_transform.sh.
// This rule strips the generated expression (and related options) while
// keeping the column definition intact so that column counts stay consistent
// between source and target databases.
type JsonGeneratedRule struct{}

// Name returns rule name
func (r *JsonGeneratedRule) Name() string { return "JsonGenerated" }

// Description returns description
func (r *JsonGeneratedRule) Description() string {
	return "Convert JSON-based generated columns to regular columns"
}

// Priority defines rule execution order
func (r *JsonGeneratedRule) Priority() int { return 700 }

// ShouldApply checks if any column in the table uses a JSON function in generated expression
func (r *JsonGeneratedRule) ShouldApply(node ast.Node) bool {
	stmt, ok := node.(*ast.CreateTableStmt)
	if !ok {
		return false
	}
	for _, col := range stmt.Cols {
		if isJsonGenerated(col) {
			return true
		}
	}
	return false
}

// Apply converts columns with JSON generated expressions into standard columns
// by removing the generated options while retaining the column and any
// associated constraints.
func (r *JsonGeneratedRule) Apply(node ast.Node) (ast.Node, error) {
	stmt := node.(*ast.CreateTableStmt)
	for _, col := range stmt.Cols {
		if !isJsonGenerated(col) {
			continue
		}
		opts := col.Options[:0]
		for _, opt := range col.Options {
			if opt.Tp == ast.ColumnOptionGenerated {
				continue
			}
			opts = append(opts, opt)
		}
		col.Options = opts
	}
	return stmt, nil
}

// isJsonGenerated checks if column has generated option with JSON function
func isJsonGenerated(col *ast.ColumnDef) bool {
	for _, opt := range col.Options {
		if opt.Tp == ast.ColumnOptionGenerated {
			if fc, ok := opt.Expr.(*ast.FuncCallExpr); ok {
				name := strings.ToLower(fc.FnName.O)
				if strings.HasPrefix(name, "json") {
					return true
				}
			}
		}
	}
	return false
}
