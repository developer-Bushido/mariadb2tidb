package rules

import (
	"github.com/pingcap/tidb/pkg/parser/ast"
)

// Rule represents a single transformation rule
type Rule interface {
	// Name returns the unique name of the rule
	Name() string

	// Description returns a human-readable description of what the rule does
	Description() string

	// Priority returns the priority of the rule (lower number = higher priority)
	Priority() int

	// ShouldApply checks if the rule should be applied to the given AST node
	ShouldApply(node ast.Node) bool

	// Apply applies the transformation to the AST node and returns the modified node
	Apply(node ast.Node) (ast.Node, error)
}

// RuleCategory represents different categories of transformation rules
type RuleCategory string

const (
	CategoryDataType      RuleCategory = "datatype"
	CategoryConstraint    RuleCategory = "constraint"
	CategoryFunction      RuleCategory = "function"
	CategoryIndex         RuleCategory = "index"
	CategoryCollation     RuleCategory = "collation"
	CategoryCompatibility RuleCategory = "compatibility"
)
