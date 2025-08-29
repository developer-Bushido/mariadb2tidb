package transformer

import (
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
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &Engine{
		registry: rules.NewRegistry(cfg),
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

		// Apply the rule via AST walking
		var err error
		result, err = e.walker.Walk(result, visitor)
		if err != nil {
			e.logger.Error("Failed to apply rule", zap.String("rule", rule.Name()), zap.Error(err))
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
}

// Enter implements parser.Visitor interface
func (v *ruleVisitor) Enter(node ast.Node) (ast.Node, bool) {
	if v.rule.ShouldApply(node) {
		v.logger.Debug("Applying rule to node",
			zap.String("rule", v.rule.Name()),
			zap.String("node", node.Text()))

		modified, err := v.rule.Apply(node)
		if err != nil {
			v.logger.Error("Rule application failed",
				zap.String("rule", v.rule.Name()),
				zap.Error(err))
			return node, false
		}

		return modified, false
	}

	return node, false
}

// Leave implements parser.Visitor interface
func (v *ruleVisitor) Leave(node ast.Node) (ast.Node, bool) {
	return node, true
}
