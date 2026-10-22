package rules

import (
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/model"
	"github.com/pingcap/tidb/pkg/parser/mysql"
)

// AutoRandomRule converts a single-column BIGINT AUTO_INCREMENT primary key
// to AUTO_RANDOM(shard_bits, range_bits) for the tables listed in
// auto_random.tables, spreading inserts across regions instead of one hot
// region. It refuses UNSIGNED columns: without a sign bit the shard bits
// land in bit 63 and IDs exceed 2^63-1 (PHP_INT_MAX, Java long).
// https://docs.pingcap.com/tidb/stable/auto-random
type AutoRandomRule struct {
	tables    map[string]bool
	all       bool
	shardBits int
	rangeBits int
	report    *compat.Reporter
}

// NewAutoRandomRule creates the rule from cfg.AutoRandom.
func NewAutoRandomRule(cfg *config.Config, r *compat.Reporter) *AutoRandomRule {
	rule := &AutoRandomRule{tables: map[string]bool{}, shardBits: 5, rangeBits: 54, report: r}
	if cfg == nil {
		return rule
	}
	for _, t := range cfg.AutoRandom.Tables {
		if t == "*" {
			rule.all = true
		}
		rule.tables[strings.ToLower(t)] = true
	}
	if cfg.AutoRandom.ShardBits > 0 {
		rule.shardBits = cfg.AutoRandom.ShardBits
	}
	if cfg.AutoRandom.RangeBits > 0 {
		rule.rangeBits = cfg.AutoRandom.RangeBits
	}
	return rule
}

// Name implements Rule.
func (r *AutoRandomRule) Name() string { return "AutoRandom" }

// Description implements Rule.
func (r *AutoRandomRule) Description() string {
	return "Convert BIGINT AUTO_INCREMENT primary keys of auto_random.tables to AUTO_RANDOM"
}

// Priority implements Rule.
func (r *AutoRandomRule) Priority() int { return 800 }

// ShouldApply implements Rule.
func (r *AutoRandomRule) ShouldApply(node ast.Node) bool {
	ct, ok := node.(*ast.CreateTableStmt)
	return ok && (r.all || r.tables[ct.Table.Name.L])
}

// Apply implements Rule.
func (r *AutoRandomRule) Apply(node ast.Node) (ast.Node, error) {
	ct := node.(*ast.CreateTableStmt)
	table := ct.Table.Name.O
	col, pk := autoIncrementPK(ct)
	switch {
	case col == nil:
		if !r.all {
			r.report.Warnf("table %s: AUTO_RANDOM skipped: needs a single-column AUTO_INCREMENT primary key", table)
		}
		return node, nil
	case col.Tp.GetType() != mysql.TypeLonglong:
		r.report.Warnf("table %s: AUTO_RANDOM skipped: %s is not BIGINT", table, col.Name.Name.O)
		return node, nil
	case mysql.HasUnsignedFlag(col.Tp.GetFlag()):
		r.report.Warnf("table %s: AUTO_RANDOM skipped: %s is UNSIGNED, IDs would exceed 2^63-1 (PHP_INT_MAX); make it signed first", table, col.Name.Name.O)
		return node, nil
	case pk != nil && pk.get() == model.PrimaryKeyTypeNonClustered:
		r.report.Warnf("table %s: AUTO_RANDOM skipped: primary key is NONCLUSTERED", table)
		return node, nil
	}

	for _, opt := range col.Options {
		if opt.Tp == ast.ColumnOptionAutoIncrement {
			opt.Tp = ast.ColumnOptionAutoRandom
			opt.AutoRandOpt = ast.AutoRandomOption{ShardBits: r.shardBits, RangeBits: r.rangeBits}
		}
	}
	if pk != nil {
		pk.set(model.PrimaryKeyTypeClustered)
	}
	r.report.Warnf("table %s: %s is AUTO_RANDOM(%d, %d) now: IDs are no longer sequential (ORDER BY id is not insertion order); loading existing rows with SQL needs SET @@allow_auto_random_explicit_insert=1 (TiDB Lightning handles it)", table, col.Name.Name.O, r.shardBits, r.rangeBits)
	if r.rangeBits > 54 {
		r.report.Warnf("table %s: AUTO_RANDOM range_bits %d allows IDs above 2^53, which JavaScript numbers cannot represent exactly", table, r.rangeBits)
	}
	return node, nil
}

// clusterHandle is where a primary key's CLUSTERED/NONCLUSTERED lives: the
// table constraint's IndexOption or the column's PRIMARY KEY option.
type clusterHandle struct {
	opt *ast.IndexOption
	col *ast.ColumnOption
}

func (h *clusterHandle) get() model.PrimaryKeyType {
	if h.opt != nil {
		return h.opt.PrimaryKeyTp
	}
	return h.col.PrimaryKeyTp
}

func (h *clusterHandle) set(t model.PrimaryKeyType) {
	if h.opt != nil {
		h.opt.PrimaryKeyTp = t
		return
	}
	h.col.PrimaryKeyTp = t
}

// autoIncrementPK returns the AUTO_INCREMENT column that alone forms the
// primary key, and a handle on the key's clustering option.
func autoIncrementPK(ct *ast.CreateTableStmt) (*ast.ColumnDef, *clusterHandle) {
	var name string
	var handle *clusterHandle
	for _, c := range ct.Constraints {
		if c.Tp == ast.ConstraintPrimaryKey && len(c.Keys) == 1 && c.Keys[0].Column != nil {
			name = c.Keys[0].Column.Name.L
			if c.Option == nil {
				c.Option = &ast.IndexOption{}
			}
			handle = &clusterHandle{opt: c.Option}
		}
	}
	for _, col := range ct.Cols {
		for _, opt := range col.Options {
			if opt.Tp == ast.ColumnOptionPrimaryKey {
				name = col.Name.Name.L
				handle = &clusterHandle{col: opt}
			}
		}
	}
	for _, col := range ct.Cols {
		if col.Name.Name.L != name || col.Tp == nil {
			continue
		}
		for _, opt := range col.Options {
			if opt.Tp == ast.ColumnOptionAutoIncrement {
				return col, handle
			}
		}
	}
	return nil, nil
}
