package rules

import (
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

// registerDefaultRules registers the default set of transformation rules,
// honoring the enabled_rules/disabled_rules configuration.
func (r *Registry) registerDefaultRules(cfg *config.Config) {
	r.logger.Info("Registering default rules")

	defaultRules := []Rule{
		// Collation rule (highest priority: charset handling affects other rules)
		NewCollationRule(cfg),
		&KeyLengthRule{},
		&IndexPrefixRule{},
		&TextBlobDefaultRule{},
		&JSONCheckRule{},
		NewFunctionDefaultRule(cfg),
		&JSONGeneratedRule{},
	}

	for _, rule := range defaultRules {
		if cfg != nil && !cfg.IsRuleEnabled(rule.Name()) {
			r.logger.Info("Skipping disabled rule", zap.String("name", rule.Name()))
			continue
		}
		r.Register(rule)
	}
}
