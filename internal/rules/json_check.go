package rules

import (
	"strings"

	"github.com/pingcap/tidb/pkg/parser/ast"
)

// JSONCheckRule removes CHECK constraints that use JSON_VALID().
// Handles both column-level and table-level check constraints.
// Reference: Step 5 in legacy universal_tidb_transform.sh
type JSONCheckRule struct{}

// Name returns rule name
func (r *JSONCheckRule) Name() string { return "JsonCheck" }

// Description returns description
func (r *JSONCheckRule) Description() string {
	return "Remove JSON_VALID check constraints"
}

// Priority defines rule execution order
func (r *JSONCheckRule) Priority() int { return 500 }

// ShouldApply determines if node contains JSON_VALID check
func (r *JSONCheckRule) ShouldApply(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.ColumnDef:
		for _, opt := range n.Options {
			if opt.Tp == ast.ColumnOptionCheck && isJSONValidExpr(opt.Expr) {
				return true
			}
		}
	case *ast.CreateTableStmt:
		for _, c := range n.Constraints {
			if c.Tp == ast.ConstraintCheck && isJSONValidExpr(c.Expr) {
				return true
			}
		}
	}
	return false
}

// Apply removes JSON_VALID check constraints
func (r *JSONCheckRule) Apply(node ast.Node) (ast.Node, error) {
	switch n := node.(type) {
	case *ast.ColumnDef:
		opts := n.Options[:0]
		for _, opt := range n.Options {
			if opt.Tp == ast.ColumnOptionCheck && isJSONValidExpr(opt.Expr) {
				continue
			}
			opts = append(opts, opt)
		}
		n.Options = opts
		return n, nil
	case *ast.CreateTableStmt:
		constraints := n.Constraints[:0]
		for _, c := range n.Constraints {
			if c.Tp == ast.ConstraintCheck && isJSONValidExpr(c.Expr) {
				continue
			}
			constraints = append(constraints, c)
		}
		n.Constraints = constraints
		return n, nil
	}
	return node, nil
}

// isJSONValidExpr checks if expression is JSON_VALID function call
func isJSONValidExpr(expr ast.ExprNode) bool {
	fc, ok := expr.(*ast.FuncCallExpr)
	if !ok {
		return false
	}
	return strings.EqualFold(fc.FnName.O, "json_valid")
}
