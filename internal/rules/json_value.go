package rules

import (
	"fmt"
	"math"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/model"
	"github.com/pingcap/tidb/pkg/parser/opcode"
)

// JSONValueRule rewrites MariaDB's JSON_VALUE(doc, path), which TiDB does
// not have (error 1305; in generated columns 3102), everywhere: generated
// columns, views, CHECK constraints, defaults. The replacement
//
//	CASE WHEN JSON_VALID(doc) AND JSON_TYPE(JSON_EXTRACT(doc, path))
//	     NOT IN ('OBJECT', 'ARRAY', 'NULL')
//	THEN JSON_UNQUOTE(JSON_EXTRACT(doc, path)) END
//
// matches MariaDB's NULL for invalid documents (a bare JSON_EXTRACT fails
// the whole INSERT with error 3140), JSON null, objects and arrays. Scalars
// can still be formatted differently (1.50 vs 1.5, true vs 1), which the
// warning says.
type JSONValueRule struct {
	report *compat.Reporter
}

// NewJSONValueRule creates the rule; r receives a one-time note on the
// remaining semantic difference.
func NewJSONValueRule(r *compat.Reporter) *JSONValueRule { return &JSONValueRule{report: r} }

// Name implements Rule.
func (r *JSONValueRule) Name() string { return "JsonValue" }

// Description implements Rule.
func (r *JSONValueRule) Description() string {
	return "Rewrite JSON_VALUE(doc, path) to JSON_UNQUOTE(JSON_EXTRACT(doc, path))"
}

// Priority implements Rule.
func (r *JSONValueRule) Priority() int { return 650 }

// ShouldApply implements Rule.
func (r *JSONValueRule) ShouldApply(node ast.Node) bool {
	fn, ok := node.(*ast.FuncCallExpr)
	return ok && fn.FnName.L == "json_value" && len(fn.Args) == 2
}

// Apply implements Rule. Nested JSON_VALUE calls in the arguments are
// rewritten too, bottom-up, because the engine does not descend into a
// node a rule replaced.
func (r *JSONValueRule) Apply(node ast.Node) (ast.Node, error) {
	r.report.Warnf("JSON_VALUE rewritten for TiDB: same NULLs for invalid JSON, null, objects and arrays; numbers and booleans may be formatted differently (1.50 -> 1.5, true -> true instead of 1), and with duplicate keys the last one wins instead of the first")
	rw := &jsonValueRewriter{depth: map[ast.Node]int{}}
	out, _ := node.Accept(rw)
	return out, rw.err
}

// maxJSONValueNesting bounds nested rewrites: each one repeats its document
// three times, so n levels repeat the innermost one 3^n times.
const maxJSONValueNesting = 4

// jsonValueRewriter replaces JSON_VALUE calls in Leave (post-order), where
// a parent accepts a node of a different type.
type jsonValueRewriter struct {
	depth map[ast.Node]int // replacement node -> nesting depth
	err   error
}

func (*jsonValueRewriter) Enter(n ast.Node) (ast.Node, bool) { return n, false }

func (rw *jsonValueRewriter) Leave(n ast.Node) (ast.Node, bool) {
	fn, ok := n.(*ast.FuncCallExpr)
	if !ok || fn.FnName.L != "json_value" || len(fn.Args) != 2 {
		return n, true
	}
	doc, path := fn.Args[0], fn.Args[1]
	depth := 1 + max(rw.nested(doc), rw.nested(path))
	if depth > maxJSONValueNesting {
		if rw.err == nil {
			rw.err = fmt.Errorf("JSON_VALUE nested %d levels deep: the TiDB rewrite would repeat the innermost document %d times; rewrite it by hand", depth, int(math.Pow(3, float64(depth))))
		}
		return n, true
	}
	call := func(name string, args ...ast.ExprNode) *ast.FuncCallExpr {
		return &ast.FuncCallExpr{FnName: model.NewCIStr(name), Args: args}
	}
	str := func(s string) ast.ExprNode { return ast.NewValueExpr(s, "", "") }
	extract := call("json_extract", doc, path)
	cond := &ast.BinaryOperationExpr{
		Op: opcode.LogicAnd,
		L:  call("json_valid", doc),
		R: &ast.PatternInExpr{
			Expr: call("json_type", extract),
			List: []ast.ExprNode{str("OBJECT"), str("ARRAY"), str("NULL")},
			Not:  true,
		},
	}
	out := &ast.CaseExpr{
		WhenClauses: []*ast.WhenClause{{Expr: cond, Result: call("json_unquote", extract)}},
	}
	rw.depth[out] = depth
	return out, true
}

// nested returns the deepest rewritten JSON_VALUE inside e.
func (rw *jsonValueRewriter) nested(e ast.Node) int {
	d := 0
	e.Accept(depthFinder{rw.depth, &d})
	return d
}

type depthFinder struct {
	depth map[ast.Node]int
	max   *int
}

func (f depthFinder) Enter(n ast.Node) (ast.Node, bool) {
	if d, ok := f.depth[n]; ok {
		*f.max = max(*f.max, d)
		return n, true
	}
	return n, false
}

func (f depthFinder) Leave(n ast.Node) (ast.Node, bool) { return n, true }
