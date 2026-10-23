package rules

import (
	"fmt"
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
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
	return NewRegistryWithReporter(cfg, nil)
}

// NewRegistryWithReporter creates a registry whose rules record migration
// warnings in r (nil disables them).
func NewRegistryWithReporter(cfg *config.Config, r *compat.Reporter) *Registry {
	registry := &Registry{
		rules:  make([]Rule, 0),
		logger: utils.GetLogger(),
	}

	registry.registerDefaultRules(cfg, r)

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
func (r *Registry) registerDefaultRules(cfg *config.Config, rep *compat.Reporter) {
	r.logger.Info("Registering default rules")

	for _, rule := range defaultRules(cfg, rep) {
		if cfg != nil && !cfg.IsRuleEnabled(rule.Name()) {
			r.logger.Info("Skipping disabled rule", zap.String("name", rule.Name()))
			continue
		}
		r.Register(rule)
	}
}

func defaultRules(cfg *config.Config, rep *compat.Reporter) []Rule {
	return []Rule{
		// Collation rule (highest priority: charset handling affects other rules)
		NewCollationRule(cfg),
		NewEngineRule(rep),
		&KeyLengthRule{},
		&IndexPrefixRule{},
		&TextBlobDefaultRule{},
		&JSONCheckRule{},
		NewFunctionDefaultRule(cfg),
		NewJSONValueRule(rep),
		NewZeroDateRule(cfg, rep),
		&JSONGeneratedRule{},
		NewAutoRandomRule(cfg, rep),
		NewShardRowIDRule(cfg),
		NewGeneratedValuesRule(rep),
	}
}

// CheckRuleNames reports rule names in enabled_rules / disabled_rules that
// do not exist: a typo in enabled_rules would otherwise silently disable
// every rule.
func CheckRuleNames(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	known := map[string]bool{}
	var names []string
	for _, rule := range defaultRules(nil, nil) {
		known[rule.Name()] = true
		names = append(names, rule.Name())
	}
	for _, list := range [][]string{cfg.EnabledRules, cfg.DisabledRules} {
		for _, n := range list {
			if !known[n] {
				return fmt.Errorf("unknown rule %q (rules: %s)", n, strings.Join(names, ", "))
			}
		}
	}
	return nil
}
