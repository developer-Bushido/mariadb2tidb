package rules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/mysql"
	"github.com/pingcap/tidb/pkg/parser/types"
)

// ZeroDateRule handles DEFAULT values such as '0000-00-00 00:00:00' on
// DATE/DATETIME/TIMESTAMP columns. MariaDB accepts them by default; TiDB
// rejects them (error 1067) under its default sql_mode, which includes
// NO_ZERO_DATE and NO_ZERO_IN_DATE. They still load where the session
// sql_mode is permissive (mariadb-dump output sets one, TiDB Lightning
// uses one), so the default is to keep them and warn; zero_date_defaults:
// drop removes the DEFAULT clause instead.
type ZeroDateRule struct {
	drop   bool
	report *compat.Reporter
}

// NewZeroDateRule creates the rule from cfg.ZeroDateDefaults.
func NewZeroDateRule(cfg *config.Config, r *compat.Reporter) *ZeroDateRule {
	return &ZeroDateRule{drop: cfg != nil && strings.EqualFold(cfg.ZeroDateDefaults, "drop"), report: r}
}

// Name implements Rule.
func (r *ZeroDateRule) Name() string { return "ZeroDate" }

// Description implements Rule.
func (r *ZeroDateRule) Description() string {
	return "Warn about (or drop, with zero_date_defaults: drop) zero-date DEFAULT values TiDB rejects in strict sql_mode"
}

// Priority implements Rule.
func (r *ZeroDateRule) Priority() int { return 450 }

// ShouldApply implements Rule.
func (r *ZeroDateRule) ShouldApply(node ast.Node) bool {
	for _, col := range columnsOf(node) {
		if zeroDateDefault(col) >= 0 {
			return true
		}
	}
	return false
}

// Apply implements Rule.
func (r *ZeroDateRule) Apply(node ast.Node) (ast.Node, error) {
	for _, col := range columnsOf(node) {
		i := zeroDateDefault(col)
		if i < 0 {
			continue
		}
		if r.drop {
			r.report.Warnf("table %s: dropped zero-date DEFAULT of %s", tableOf(node), col.Name.Name.O)
			col.Options = append(col.Options[:i], col.Options[i+1:]...)
			continue
		}
		r.report.Warnf("table %s: column %s has a zero-date DEFAULT; TiDB rejects it under the default sql_mode (NO_ZERO_DATE/NO_ZERO_IN_DATE): apply the DDL with a permissive sql_mode (TiDB Lightning does) or set zero_date_defaults: drop", tableOf(node), col.Name.Name.O)
	}
	return node, nil
}

// columnsOf returns the column definitions of a CREATE TABLE or of the
// ADD/MODIFY/CHANGE specs of an ALTER TABLE.
func columnsOf(node ast.Node) []*ast.ColumnDef {
	switch n := node.(type) {
	case *ast.CreateTableStmt:
		return n.Cols
	case *ast.AlterTableStmt:
		var cols []*ast.ColumnDef
		for _, spec := range n.Specs {
			cols = append(cols, spec.NewColumns...)
		}
		return cols
	}
	return nil
}

// zeroDateDefault returns the index of a zero-date DEFAULT option of a
// date/time column, or -1.
func zeroDateDefault(col *ast.ColumnDef) int {
	if col.Tp == nil {
		return -1
	}
	switch col.Tp.GetType() {
	case mysql.TypeDate, mysql.TypeDatetime, mysql.TypeTimestamp:
	default:
		return -1
	}
	for i, opt := range col.Options {
		if opt.Tp != ast.ColumnOptionDefaultValue {
			continue
		}
		v, ok := opt.Expr.(ast.ValueExpr)
		if !ok {
			continue
		}
		switch v.GetType().EvalType() {
		case types.ETString:
			if isZeroDate(v.GetString()) {
				return i
			}
		case types.ETInt, types.ETDecimal, types.ETReal:
			// DEFAULT 0, DEFAULT 20200010: numbers are read as YYYYMMDD[...].
			if isZeroDate(strings.TrimLeft(fmt.Sprint(v.GetValue()), "-")) {
				return i
			}
		}
	}
	return -1
}

// isZeroDate reports a date literal TiDB rejects under NO_ZERO_DATE /
// NO_ZERO_IN_DATE: a zero month or day. It reads the forms MySQL accepts:
// delimited ('0000-00-00', '0000/00/00 00:00:00'), digits only
// ('00000000000000', '20200010', six-digit YYMMDD) and '0'. Year 0 alone
// ('0000-01-01') is valid.
func isZeroDate(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	digitsOnly := strings.Trim(s, "0123456789.") == ""
	var month, day string
	switch {
	case digitsOnly:
		s = strings.SplitN(s, ".", 2)[0]
		switch {
		case strings.Trim(s, "0") == "":
			return true
		case len(s) >= 8:
			month, day = s[4:6], s[6:8]
		case len(s) == 6:
			month, day = s[2:4], s[4:6]
		default:
			return false
		}
	default:
		parts := strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' })
		if len(parts) < 3 {
			return false
		}
		month, day = parts[1], parts[2]
	}
	m, err1 := strconv.Atoi(month)
	d, err2 := strconv.Atoi(day)
	return err1 == nil && err2 == nil && (m == 0 || d == 0)
}
