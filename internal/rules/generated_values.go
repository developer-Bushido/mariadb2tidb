package rules

import (
	"strings"
	"sync"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/pingcap/tidb/pkg/parser/ast"
)

// GeneratedValuesRule replaces values given for generated columns in
// INSERT/REPLACE with DEFAULT. mariadb-dump writes them (MariaDB ignores
// them on load); TiDB rejects the statement with error 3105. Generated
// columns are learned from CREATE TABLE statements earlier in the same
// input, so this covers full dumps (schema and data in one file). Tables are
// keyed by database, following USE, so equal names in different databases
// do not mix.
type GeneratedValuesRule struct {
	report *compat.Reporter

	mu     sync.Mutex
	db     string                   // current database (USE)
	tables map[string]*tableColumns // "db.table" -> columns
}

type tableColumns struct {
	names     []string
	generated []bool
}

// NewGeneratedValuesRule creates the rule.
func NewGeneratedValuesRule(r *compat.Reporter) *GeneratedValuesRule {
	return &GeneratedValuesRule{report: r, tables: map[string]*tableColumns{}}
}

// Name implements Rule.
func (r *GeneratedValuesRule) Name() string { return "GeneratedValues" }

// Description implements Rule.
func (r *GeneratedValuesRule) Description() string {
	return "Replace values for generated columns in INSERT/REPLACE with DEFAULT"
}

// Priority implements Rule.
func (r *GeneratedValuesRule) Priority() int { return 900 }

// ShouldApply implements Rule.
func (r *GeneratedValuesRule) ShouldApply(node ast.Node) bool {
	switch node.(type) {
	case *ast.UseStmt, *ast.CreateTableStmt, *ast.InsertStmt:
		return true
	}
	return false
}

// Apply implements Rule.
func (r *GeneratedValuesRule) Apply(node ast.Node) (ast.Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch n := node.(type) {
	case *ast.UseStmt:
		r.db = strings.ToLower(n.DBName)
	case *ast.CreateTableStmt:
		tc := &tableColumns{}
		for _, col := range n.Cols {
			gen := false
			for _, opt := range col.Options {
				gen = gen || opt.Tp == ast.ColumnOptionGenerated
			}
			tc.names = append(tc.names, col.Name.Name.L)
			tc.generated = append(tc.generated, gen)
		}
		r.tables[r.key(n.Table)] = tc
	case *ast.InsertStmt:
		r.rewriteInsert(n)
	}
	return node, nil
}

func (r *GeneratedValuesRule) key(t *ast.TableName) string {
	db := t.Schema.L
	if db == "" {
		db = r.db
	}
	return db + "." + t.Name.L
}

func (r *GeneratedValuesRule) rewriteInsert(n *ast.InsertStmt) {
	tn := insertTable(n)
	if tn == nil {
		return
	}
	table := tn.Name.O
	tc, ok := r.tables[r.key(tn)]
	if !ok {
		return
	}
	index := map[string]int{}
	for i, name := range tc.names {
		index[name] = i
	}
	isGenerated := func(c *ast.ColumnName) bool {
		j, ok := index[c.Name.L]
		return ok && tc.generated[j]
	}

	// Positions of the generated columns among the statement's values.
	var pos []int
	var cols []string
	if len(n.Columns) == 0 {
		for i, g := range tc.generated {
			if g {
				pos = append(pos, i)
				cols = append(cols, tc.names[i])
			}
		}
	} else {
		for i, c := range n.Columns {
			if isGenerated(c) {
				pos = append(pos, i)
				cols = append(cols, c.Name.L)
			}
		}
	}
	if len(pos) > 0 && n.Select != nil {
		r.report.Warnf("table %s: INSERT ... SELECT writes generated columns %s; TiDB rejects it (error 3105), list only the other columns", table, strings.Join(cols, ", "))
	}

	changed := false
	for _, row := range n.Lists {
		for _, p := range pos {
			if p < len(row) {
				if _, isDefault := row[p].(*ast.DefaultExpr); !isDefault {
					row[p] = &ast.DefaultExpr{}
					changed = true
				}
			}
		}
	}
	if changed {
		r.report.Warnf("table %s: values for generated columns %s replaced with DEFAULT (TiDB computes them and rejects given values)", table, strings.Join(cols, ", "))
	}

	// ON DUPLICATE KEY UPDATE b = VALUES(b) on a generated column is
	// rejected as well; without it the update keeps its other assignments.
	var kept []*ast.Assignment
	for _, a := range n.OnDuplicate {
		if isGenerated(a.Column) {
			continue
		}
		kept = append(kept, a)
	}
	switch {
	case len(kept) == len(n.OnDuplicate):
	case len(kept) == 0:
		r.report.Warnf("table %s: ON DUPLICATE KEY UPDATE only assigns generated columns; TiDB rejects it (error 3105)", table)
	default:
		n.OnDuplicate = kept
		r.report.Warnf("table %s: ON DUPLICATE KEY UPDATE of generated columns dropped", table)
	}
}

// insertTable returns the target table of an INSERT.
func insertTable(n *ast.InsertStmt) *ast.TableName {
	if n.Table == nil || n.Table.TableRefs == nil {
		return nil
	}
	if ts, ok := n.Table.TableRefs.Left.(*ast.TableSource); ok {
		if tn, ok := ts.Source.(*ast.TableName); ok {
			return tn
		}
	}
	return nil
}
