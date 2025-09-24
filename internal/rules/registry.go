package rules

import (
	"sort"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
	"go.uber.org/zap"
)

// Registry manages all transformation rules
type Registry struct {
	rules  []Rule
	logger *zap.Logger
}

// NewRegistry creates a new rules registry
func NewRegistry(cfg *config.Config) *Registry {
	registry := &Registry{
		rules:  make([]Rule, 0),
		logger: utils.GetLogger(),
	}

	// Register default rules (currently empty stubs)
	registry.registerDefaultRules(cfg)

	return registry
}

// Register adds a rule to the registry
func (r *Registry) Register(rule Rule) {
	r.logger.Info("Registering rule", zap.String("name", rule.Name()), zap.Int("priority", rule.Priority()))

	// Insert rule in priority order
	inserted := false
	for i, existingRule := range r.rules {
		if rule.Priority() < existingRule.Priority() {
			// Insert at position i
			r.rules = append(r.rules[:i], append([]Rule{rule}, r.rules[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		r.rules = append(r.rules, rule)
	}
}

// GetRules returns all registered rules in priority order
func (r *Registry) GetRules() []Rule {
	// Return a copy to prevent external modification
	result := make([]Rule, len(r.rules))
	copy(result, r.rules)
	return result
}

// GetRuleByName returns a rule by its name
func (r *Registry) GetRuleByName(name string) Rule {
	for _, rule := range r.rules {
		if rule.Name() == name {
			return rule
		}
	}
	return nil
}

// ListRules returns information about all registered rules
func (r *Registry) ListRules() []RuleInfo {
	result := make([]RuleInfo, len(r.rules))
	for i, rule := range r.rules {
		result[i] = RuleInfo{
			Name:        rule.Name(),
			Description: rule.Description(),
			Priority:    rule.Priority(),
		}
	}
	return result
}

// RuleInfo contains metadata about a rule
type RuleInfo struct {
	Name        string
	Description string
	Priority    int
}

// registerDefaultRules registers the default set of transformation rules
func (r *Registry) registerDefaultRules(cfg *config.Config) {
	r.logger.Info("Registering default rules")

	// For now, register empty stubs for all planned rules
	// In future iterations, these will be replaced with actual implementations

	defaultRules := []Rule{
		// T-0002: Collation rule (highest priority for charset handling)
		NewCollationRule(cfg),
		// T-0004: KeyLength rule
		&KeyLengthRule{},
		// Handle missing prefix lengths on indexed text columns
		&IndexPrefixRule{},
		&TextBlobDefaultRule{},
		&JSONCheckRule{},
		&FunctionDefaultRule{},
		&JSONGeneratedRule{},
	}

	for _, rule := range defaultRules {
		r.Register(rule)
	}

	// Sort rules by priority to ensure correct order
	sort.Slice(r.rules, func(i, j int) bool {
		return r.rules[i].Priority() < r.rules[j].Priority()
	})
}
