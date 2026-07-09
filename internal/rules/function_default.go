package rules

import (
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/pingcap/tidb/pkg/parser/ast"
)

// FunctionDefaultRule removes unsupported function-based default values.
// TiDB allows only a small set of functions in DEFAULT clauses without the
// expression syntax (for example, CURRENT_TIMESTAMP); newer TiDB versions
// accept more expressions, so the allowlist is configurable via
// allowed_default_functions.
// https://docs.pingcap.com/tidb/stable/data-type-default-values
type FunctionDefaultRule struct {
	allowed map[string]bool
}

// defaultAllowedDefaultFuncs lists function names every supported TiDB
// version accepts in DEFAULT clauses.
var defaultAllowedDefaultFuncs = []string{
	"current_timestamp",
	"current_date",
	"current_time",
	"now",
	"localtime",
	"localtimestamp",
}

// NewFunctionDefaultRule creates the rule using the allowlist from cfg,
// falling back to the built-in defaults.
func NewFunctionDefaultRule(cfg *config.Config) *FunctionDefaultRule {
	names := defaultAllowedDefaultFuncs
	if cfg != nil && len(cfg.AllowedDefaultFunctions) > 0 {
		names = cfg.AllowedDefaultFunctions
	}
	allowed := make(map[string]bool, len(names))
	for _, n := range names {
		allowed[strings.ToLower(n)] = true
	}
	return &FunctionDefaultRule{allowed: allowed}
}

// Name returns rule name
func (r *FunctionDefaultRule) Name() string { return "FunctionDefault" }

// Description returns rule description
func (r *FunctionDefaultRule) Description() string {
	return "Remove unsupported function-based default values"
}

// Priority defines rule execution order
func (r *FunctionDefaultRule) Priority() int { return 600 }

// ShouldApply checks if the column has a function-based default that TiDB disallows
func (r *FunctionDefaultRule) ShouldApply(node ast.Node) bool {
	col, ok := node.(*ast.ColumnDef)
	if !ok {
		return false
	}
	for _, opt := range col.Options {
		if opt.Tp == ast.ColumnOptionDefaultValue {
			if fc, ok := opt.Expr.(*ast.FuncCallExpr); ok {
				if !r.isAllowed(fc.FnName.O) {
					return true
				}
			}
		}
	}
	return false
}

// Apply removes the default clause if it uses a disallowed function
func (r *FunctionDefaultRule) Apply(node ast.Node) (ast.Node, error) {
	col, ok := node.(*ast.ColumnDef)
	if !ok {
		return node, nil
	}
	opts := col.Options[:0]
	for _, opt := range col.Options {
		if opt.Tp == ast.ColumnOptionDefaultValue {
			if fc, ok := opt.Expr.(*ast.FuncCallExpr); ok {
				if !r.isAllowed(fc.FnName.O) {
					continue
				}
			}
		}
		opts = append(opts, opt)
	}
	col.Options = opts
	return col, nil
}

// isAllowed reports whether a function name may stay in a DEFAULT clause.
// A zero-value rule falls back to the built-in allowlist.
func (r *FunctionDefaultRule) isAllowed(name string) bool {
	name = strings.ToLower(name)
	if r.allowed != nil {
		return r.allowed[name]
	}
	for _, n := range defaultAllowedDefaultFuncs {
		if n == name {
			return true
		}
	}
	return false
}
