package transformer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
)

// Shape is what no transformation may change, statement by statement and in
// order: tables with their columns, keys (including column-level ones, key
// order and DESC, foreign key targets and actions) and partitioning; views;
// INSERTs with their target, columns and row/value counts; ALTER TABLE
// specs; USE. Rules legitimately change types, defaults, options, key
// prefix lengths, clustering and CHECK constraints (MariaDB's JSON_VALID
// checks are dropped on purpose), so those are not part of it.
type Shape []string

// ShapeOf records the shape of stmts. Take it before Transform: rules
// modify the AST in place.
func ShapeOf(stmts []ast.StmtNode) Shape {
	s := make(Shape, len(stmts))
	for i, stmt := range stmts {
		s[i] = signature(stmt)
	}
	return s
}

// VerifyOutput re-parses output with the TiDB parser and checks it still has
// the shape recorded before the transformation, so a bug anywhere in the
// pipeline cannot silently drop a table, column, key, row or statement.
func VerifyOutput(before Shape, output string) error {
	stmts, err := parser.NewLoader().ParseTiDB(output)
	if err != nil {
		return fmt.Errorf("output check: output does not parse: %w", err)
	}
	after := ShapeOf(stmts)
	if len(after) != len(before) {
		return fmt.Errorf("output check: %d statements in input, %d in output", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			return fmt.Errorf("output check: statement %d changed:\n  input:  %s\n  output: %s", i+1, before[i], after[i])
		}
	}
	return nil
}

func signature(stmt ast.StmtNode) string {
	switch n := stmt.(type) {
	case *ast.CreateTableStmt:
		return createTableSignature(n)
	case *ast.AlterTableStmt:
		var specs []string
		for _, spec := range n.Specs {
			if spec.Tp == ast.AlterTableOption {
				continue // table options: rules change them on purpose
			}
			parts := []string{fmt.Sprint(spec.Tp)}
			for _, c := range spec.NewColumns {
				parts = append(parts, c.Name.Name.L)
				parts = append(parts, columnKeys(c)...)
			}
			if spec.Constraint != nil && spec.Constraint.Tp != ast.ConstraintCheck {
				parts = append(parts, constraintSignature(spec.Constraint))
			}
			if spec.OldColumnName != nil {
				parts = append(parts, spec.OldColumnName.Name.L)
			}
			if spec.NewColumnName != nil {
				parts = append(parts, "->"+spec.NewColumnName.Name.L)
			}
			if spec.Name != "" {
				parts = append(parts, strings.ToLower(spec.Name))
			}
			if spec.FromKey.L != "" || spec.ToKey.L != "" {
				parts = append(parts, spec.FromKey.L+"->"+spec.ToKey.L)
			}
			if spec.NewTable != nil {
				parts = append(parts, "to "+tableName(spec.NewTable))
			}
			specs = append(specs, strings.Join(parts, " "))
		}
		return fmt.Sprintf("ALTER TABLE %s [%s]", tableName(n.Table), strings.Join(specs, "; "))
	case *ast.InsertStmt:
		target := "?"
		if ts, ok := n.Table.TableRefs.Left.(*ast.TableSource); ok {
			if tn, ok := ts.Source.(*ast.TableName); ok {
				target = tableName(tn)
			}
		}
		var cols []string
		for _, c := range n.Columns {
			cols = append(cols, c.Name.L)
		}
		var rows []string
		for _, row := range n.Lists {
			rows = append(rows, fmt.Sprint(len(row)))
		}
		return fmt.Sprintf("INSERT replace=%v ignore=%v %s (%s) rows=[%s] select=%v",
			n.IsReplace, n.IgnoreErr, target, strings.Join(cols, ","), strings.Join(rows, ","), n.Select != nil)
	case *ast.CreateViewStmt:
		var cols []string
		for _, c := range n.Cols {
			cols = append(cols, c.L)
		}
		return fmt.Sprintf("CREATE VIEW %s (%s)", tableName(n.ViewName), strings.Join(cols, ","))
	case *ast.CreateIndexStmt:
		var parts []string
		for _, k := range n.IndexPartSpecifications {
			p := "(expr)"
			if k.Column != nil {
				p = k.Column.Name.L
			}
			if k.Desc {
				p += " desc"
			}
			parts = append(parts, p)
		}
		return fmt.Sprintf("CREATE INDEX %d %s ON %s (%s)", n.KeyType, strings.ToLower(n.IndexName), tableName(n.Table), strings.Join(parts, ","))
	case *ast.DropIndexStmt:
		return fmt.Sprintf("DROP INDEX %s ON %s", strings.ToLower(n.IndexName), tableName(n.Table))
	case *ast.RenameTableStmt:
		var pairs []string
		for _, p := range n.TableToTables {
			pairs = append(pairs, tableName(p.OldTable)+"->"+tableName(p.NewTable))
		}
		return "RENAME TABLE " + strings.Join(pairs, ",")
	case *ast.UseStmt:
		return "USE " + strings.ToLower(n.DBName)
	case *ast.CreateDatabaseStmt:
		return "CREATE DATABASE " + n.Name.L
	case *ast.DropTableStmt:
		var names []string
		for _, t := range n.Tables {
			names = append(names, tableName(t))
		}
		return fmt.Sprintf("DROP view=%v %s", n.IsView, strings.Join(names, ","))
	case *ast.CreateSequenceStmt:
		return "CREATE SEQUENCE " + tableName(n.Name)
	}
	return fmt.Sprintf("%T", stmt)
}

func createTableSignature(n *ast.CreateTableStmt) string {
	var cols, keys []string
	for _, col := range n.Cols {
		cols = append(cols, col.Name.Name.L)
		keys = append(keys, columnKeys(col)...)
	}
	for _, c := range n.Constraints {
		if c.Tp != ast.ConstraintCheck {
			keys = append(keys, constraintSignature(c))
		}
	}
	sort.Strings(keys)
	var like, part string
	if n.ReferTable != nil {
		like = " like=" + tableName(n.ReferTable)
	}
	if n.Partition != nil {
		part = fmt.Sprintf(" partition=%d/%d", n.Partition.Tp, len(n.Partition.Definitions))
	}
	return fmt.Sprintf("CREATE TABLE %s (%s) keys=[%s]%s%s select=%v",
		tableName(n.Table), strings.Join(cols, ","), strings.Join(keys, "; "), like, part, n.Select != nil)
}

// columnKeys returns the keys declared on a column (PRIMARY KEY, UNIQUE,
// REFERENCES).
func columnKeys(col *ast.ColumnDef) []string {
	var keys []string
	for _, opt := range col.Options {
		switch opt.Tp {
		case ast.ColumnOptionPrimaryKey:
			keys = append(keys, "primary("+col.Name.Name.L+")")
		case ast.ColumnOptionUniqKey:
			keys = append(keys, "unique("+col.Name.Name.L+")")
		case ast.ColumnOptionReference:
			keys = append(keys, "references("+col.Name.Name.L+")->"+referenceSignature(opt.Refer))
		}
	}
	return keys
}

func constraintSignature(c *ast.Constraint) string {
	var parts []string
	for _, k := range c.Keys {
		p := "(expr)"
		if k.Column != nil {
			p = k.Column.Name.L
		}
		if k.Desc {
			p += " desc"
		}
		parts = append(parts, p)
	}
	sig := fmt.Sprintf("%d %s(%s)", c.Tp, strings.ToLower(c.Name), strings.Join(parts, ","))
	if c.Refer != nil {
		sig += "->" + referenceSignature(c.Refer)
	}
	return sig
}

func referenceSignature(r *ast.ReferenceDef) string {
	if r == nil {
		return ""
	}
	var cols []string
	for _, k := range r.IndexPartSpecifications {
		if k.Column != nil {
			cols = append(cols, k.Column.Name.L)
		}
	}
	sig := tableName(r.Table) + "(" + strings.Join(cols, ",") + ")"
	if r.OnDelete != nil {
		sig += fmt.Sprintf(" delete=%d", r.OnDelete.ReferOpt)
	}
	if r.OnUpdate != nil {
		sig += fmt.Sprintf(" update=%d", r.OnUpdate.ReferOpt)
	}
	return sig
}

func tableName(t *ast.TableName) string {
	if t == nil {
		return "?"
	}
	if t.Schema.L == "" {
		return t.Name.L
	}
	return t.Schema.L + "." + t.Name.L
}
