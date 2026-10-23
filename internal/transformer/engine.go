package transformer

import (
	"fmt"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/parser"
	"github.com/developer-Bushido/mariadb2tidb/internal/rules"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"go.uber.org/zap"
)

// Engine handles the transformation of SQL statements
type Engine struct {
	registry *rules.Registry
	walker   *parser.Walker
	logger   *zap.Logger
}

// NewEngine creates a new transformation engine
func NewEngine(cfg *config.Config) *Engine {
	return NewEngineWithReporter(cfg, nil)
}

// NewEngineWithReporter creates an engine whose rules record migration
// warnings in r.
func NewEngineWithReporter(cfg *config.Config, r *compat.Reporter) *Engine {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &Engine{
		registry: rules.NewRegistryWithReporter(cfg, r),
		walker:   parser.NewWalker(),
		logger:   utils.GetLogger(),
	}
}

// Transform applies all registered rules to the SQL statements
func (e *Engine) Transform(stmts []ast.StmtNode) ([]ast.StmtNode, error) {
	e.logger.Info("Starting transformation", zap.Int("statements", len(stmts)))

	result := stmts

	// Get all rules in priority order
	ruleList := e.registry.GetRules()

	for _, rule := range ruleList {
		e.logger.Debug("Applying rule", zap.String("name", rule.Name()))

		// Create a visitor for this rule
		visitor := &ruleVisitor{
			rule:   rule,
			logger: e.logger,
		}

		// Apply the rule via AST walking. A failing rule fails the whole
		// transformation: silently keeping the node would emit DDL the rule
		// was meant to fix.
		var err error
		result, err = e.walker.Walk(result, visitor)
		if err == nil {
			err = visitor.err
		}
		if err != nil {
			e.logger.Debug("Failed to apply rule", zap.String("rule", rule.Name()), zap.Error(err))
			return nil, err
		}

		e.logger.Debug("Rule applied successfully", zap.String("name", rule.Name()))
	}

	e.logger.Info("Transformation completed", zap.Int("statements", len(result)))
	return result, nil
}

// ruleVisitor implements parser.Visitor to apply a single rule
type ruleVisitor struct {
	rule   rules.Rule
	logger *zap.Logger
	err    error // first Apply error
}

// Enter implements parser.Visitor interface
func (v *ruleVisitor) Enter(node ast.Node) (ast.Node, bool) {
	if v.rule.ShouldApply(node) {
		v.logger.Debug("Applying rule to node",
			zap.String("rule", v.rule.Name()),
			zap.String("node", node.Text()))

		modified, err := v.rule.Apply(node)
		if err != nil {
			if v.err == nil {
				v.err = fmt.Errorf("rule %s: %w", v.rule.Name(), err)
			}
			return node, true
		}

		// A replacement node may have another type, which TiDB's Accept
		// only takes when the children are skipped; the rule handled them.
		return modified, modified != node
	}

	return node, false
}

// Leave implements parser.Visitor interface
func (v *ruleVisitor) Leave(node ast.Node) (ast.Node, bool) {
	return node, true
}
