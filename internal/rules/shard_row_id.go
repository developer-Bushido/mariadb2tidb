package rules

import (
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/model"
)

// ShardRowIDRule adds SHARD_ROW_ID_BITS (and optionally PRE_SPLIT_REGIONS)
// to tables whose rows are keyed by the hidden _tidb_rowid: tables without
// a PRIMARY KEY, or with a NONCLUSTERED one. Every other primary key is
// clustered by default (tidb_enable_clustered_index=ON) and TiDB rejects
// the option there (error 8200). Opt-in via shard_row_id_bits.
// https://docs.pingcap.com/tidb/stable/shard-row-id-bits
type ShardRowIDRule struct {
	bits, preSplit uint64
}

// NewShardRowIDRule creates the rule from cfg.
func NewShardRowIDRule(cfg *config.Config) *ShardRowIDRule {
	if cfg == nil {
		return &ShardRowIDRule{}
	}
	return &ShardRowIDRule{bits: cfg.ShardRowIDBits, preSplit: cfg.PreSplitRegions}
}

// Name implements Rule.
func (r *ShardRowIDRule) Name() string { return "ShardRowID" }

// Description implements Rule.
func (r *ShardRowIDRule) Description() string {
	return "Add SHARD_ROW_ID_BITS to tables without a clustered primary key"
}

// Priority implements Rule.
func (r *ShardRowIDRule) Priority() int { return 810 }

// ShouldApply implements Rule.
func (r *ShardRowIDRule) ShouldApply(node ast.Node) bool {
	ct, ok := node.(*ast.CreateTableStmt)
	// TiDB rejects the option on temporary tables (error 8006).
	if !ok || r.bits == 0 || ct.ReferTable != nil || ct.Select != nil || ct.TemporaryKeyword != ast.TemporaryNone || hasClusteredPK(ct) {
		return false
	}
	for _, opt := range ct.Options {
		if opt.Tp == ast.TableOptionShardRowID {
			return false
		}
	}
	return true
}

// Apply implements Rule.
func (r *ShardRowIDRule) Apply(node ast.Node) (ast.Node, error) {
	ct := node.(*ast.CreateTableStmt)
	ct.Options = append(ct.Options, &ast.TableOption{Tp: ast.TableOptionShardRowID, UintValue: r.bits})
	if r.preSplit > 0 {
		ct.Options = append(ct.Options, &ast.TableOption{Tp: ast.TableOptionPreSplitRegion, UintValue: r.preSplit})
	}
	return node, nil
}

// hasClusteredPK reports a primary key that is not explicitly NONCLUSTERED.
func hasClusteredPK(ct *ast.CreateTableStmt) bool {
	for _, c := range ct.Constraints {
		if c.Tp == ast.ConstraintPrimaryKey {
			return c.Option == nil || c.Option.PrimaryKeyTp != model.PrimaryKeyTypeNonClustered
		}
	}
	for _, col := range ct.Cols {
		for _, opt := range col.Options {
			if opt.Tp == ast.ColumnOptionPrimaryKey {
				return opt.PrimaryKeyTp != model.PrimaryKeyTypeNonClustered
			}
		}
	}
	return false
}
