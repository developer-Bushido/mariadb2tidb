package rules

import (
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/pingcap/tidb/pkg/parser/ast"
)

// supportedEngines are the ENGINE values TiDB v8.5 accepts (and ignores:
// TiDB always stores data in TiKV). Anything else fails with error 1286.
var supportedEngines = map[string]bool{
	"innodb": true, "myisam": true, "memory": true, "archive": true,
	"csv": true, "blackhole": true, "federated": true, "merge": true,
	"heap": true, "ndb": true, "example": true,
}

// EngineRule replaces storage engines TiDB rejects (MariaDB's Aria,
// ColumnStore, Spider, CONNECT, S3, ROCKSDB, MRG_MyISAM, ...) with InnoDB.
type EngineRule struct {
	report *compat.Reporter
}

// NewEngineRule creates the rule; r receives a warning per replacement.
func NewEngineRule(r *compat.Reporter) *EngineRule { return &EngineRule{report: r} }

// Name implements Rule.
func (r *EngineRule) Name() string { return "Engine" }

// Description implements Rule.
func (r *EngineRule) Description() string {
	return "Replace storage engines TiDB does not accept (Aria, ColumnStore, ...) with InnoDB"
}

// Priority implements Rule.
func (r *EngineRule) Priority() int { return 150 }

// ShouldApply implements Rule.
func (r *EngineRule) ShouldApply(node ast.Node) bool {
	for _, opts := range mergeOptionLists(node) {
		if hasMergeOptions(*opts) {
			return true
		}
	}
	return len(r.unsupported(node)) > 0
}

// Apply implements Rule.
func (r *EngineRule) Apply(node ast.Node) (ast.Node, error) {
	for _, opt := range r.unsupported(node) {
		r.report.Warnf("table %s: ENGINE=%s replaced with InnoDB (not supported by TiDB)", tableOf(node), opt.StrValue)
		opt.StrValue = "InnoDB"
	}
	// MERGE tables' UNION=(...) and INSERT_METHOD are rejected by TiDB
	// (errors 8232, 8233) whatever the engine.
	for _, opts := range mergeOptionLists(node) {
		if !hasMergeOptions(*opts) {
			continue
		}
		r.report.Warnf("table %s: MERGE table became an ordinary empty table (TiDB rejects UNION and INSERT_METHOD); its UNION tables are not merged", tableOf(node))
		var kept []*ast.TableOption
		for _, opt := range *opts {
			if opt.Tp != ast.TableOptionUnion && opt.Tp != ast.TableOptionInsertMethod {
				kept = append(kept, opt)
			}
		}
		*opts = kept
	}
	if at, ok := node.(*ast.AlterTableStmt); ok {
		var specs []*ast.AlterTableSpec
		for _, spec := range at.Specs {
			if spec.Tp != ast.AlterTableOption || len(spec.Options) > 0 {
				specs = append(specs, spec)
			}
		}
		at.Specs = specs
	}
	return node, nil
}

// mergeOptionLists returns the table option lists of a CREATE TABLE or of
// each ALTER TABLE spec.
func mergeOptionLists(node ast.Node) []*[]*ast.TableOption {
	switch n := node.(type) {
	case *ast.CreateTableStmt:
		return []*[]*ast.TableOption{&n.Options}
	case *ast.AlterTableStmt:
		var lists []*[]*ast.TableOption
		for _, spec := range n.Specs {
			lists = append(lists, &spec.Options)
		}
		return lists
	}
	return nil
}

func hasMergeOptions(opts []*ast.TableOption) bool {
	for _, opt := range opts {
		if opt.Tp == ast.TableOptionUnion || opt.Tp == ast.TableOptionInsertMethod {
			return true
		}
	}
	return false
}

func (r *EngineRule) unsupported(node ast.Node) []*ast.TableOption {
	var opts []*ast.TableOption
	switch n := node.(type) {
	case *ast.CreateTableStmt:
		opts = n.Options
	case *ast.AlterTableStmt:
		for _, spec := range n.Specs {
			opts = append(opts, spec.Options...)
		}
	}
	var bad []*ast.TableOption
	for _, opt := range opts {
		if opt.Tp == ast.TableOptionEngine && opt.StrValue != "" && !supportedEngines[strings.ToLower(opt.StrValue)] {
			bad = append(bad, opt)
		}
	}
	return bad
}

// tableOf returns the table name of a CREATE/ALTER TABLE for messages.
func tableOf(node ast.Node) string {
	switch n := node.(type) {
	case *ast.CreateTableStmt:
		return n.Table.Name.O
	case *ast.AlterTableStmt:
		return n.Table.Name.O
	}
	return "?"
}
