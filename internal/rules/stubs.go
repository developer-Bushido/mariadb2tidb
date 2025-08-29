package rules

import (
	"github.com/pingcap/tidb/pkg/parser/ast"
)

// StubRule provides a base implementation for rules that are not yet implemented
type StubRule struct {
	name        string
	description string
	priority    int
}

// NewStubRule creates a new stub rule
func NewStubRule(name, description string, priority int) *StubRule {
	return &StubRule{
		name:        name,
		description: description,
		priority:    priority,
	}
}

// Name implements Rule interface
func (r *StubRule) Name() string {
	return r.name
}

// Description implements Rule interface
func (r *StubRule) Description() string {
	return r.description + " (stub implementation)"
}

// Priority implements Rule interface
func (r *StubRule) Priority() int {
	return r.priority
}

// ShouldApply implements Rule interface - stub always returns false
func (r *StubRule) ShouldApply(node ast.Node) bool {
	return false // Stub rules don't apply to anything
}

// Apply implements Rule interface - stub returns node unchanged
func (r *StubRule) Apply(node ast.Node) (ast.Node, error) {
	return node, nil // Stub rules don't modify anything
}

// CreateStubRules creates stub implementations for all planned rules
func CreateStubRules() []Rule {
	return []Rule{
		NewStubRule("Collation", "Transform MariaDB collations to TiDB compatible ones", 10),
		NewStubRule("KeyLength", "Remove or adjust key length limits", 20),
		NewStubRule("IntegerWidth", "Remove integer display width specifications", 30),
		NewStubRule("TextBlobDefaults", "Remove default values from TEXT/BLOB columns", 40),
		NewStubRule("JsonCheck", "Transform JSON check constraints", 50),
		NewStubRule("FunctionDefault", "Transform function-based default values", 60),
		NewStubRule("ZeroTimestamp", "Handle zero timestamp values", 70),
		NewStubRule("UUIDType", "Transform UUID data types", 80),
		NewStubRule("MariaDBSpecific", "Remove MariaDB-specific syntax", 90),
		NewStubRule("Constraints", "Transform constraint definitions", 100),
		NewStubRule("TrailingComma", "Remove trailing commas", 110),
		NewStubRule("VersionMacros", "Transform version-specific macros", 130),
		NewStubRule("AutoIncrementValues", "Adjust auto-increment starting values", 140),
		NewStubRule("OnUpdateCurrentTimestamp", "Transform ON UPDATE CURRENT_TIMESTAMP", 150),
		NewStubRule("IndexType", "Transform index type specifications", 160),
		NewStubRule("QualifiedNames", "Handle qualified table/column names", 170),
	}
}
