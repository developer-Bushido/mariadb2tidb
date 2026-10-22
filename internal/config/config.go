// Package config defines the YAML-backed configuration for the
// transformation engine: directories, rule toggles, and charset/collation
// mappings.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// Config holds configuration for the transformation engine
type Config struct {
	// InputDir specifies the directory to read SQL files from
	InputDir string `yaml:"input_dir"`

	// OutputDir specifies the directory to write transformed SQL files
	OutputDir string `yaml:"output_dir"`

	// EnabledRules is a list of rule names to enable
	EnabledRules []string `yaml:"enabled_rules"`

	// DisabledRules is a list of rule names to disable
	DisabledRules []string `yaml:"disabled_rules"`

	// FormattingOptions controls output formatting
	FormattingOptions FormattingConfig `yaml:"formatting"`

	// CharsetMappings defines how to convert character sets
	CharsetMappings map[string]CharsetMapping `yaml:"charset_mappings"`

	// CollationMappings defines how to convert collations
	CollationMappings map[string]string `yaml:"collation_mappings"`

	// AllowedDefaultFunctions lists function names the FunctionDefault rule
	// keeps in DEFAULT clauses; anything else is stripped.
	// https://docs.pingcap.com/tidb/stable/data-type-default-values
	AllowedDefaultFunctions []string `yaml:"allowed_default_functions"`

	// ZeroDateDefaults controls DEFAULT '0000-00-00...' on date/time columns,
	// which TiDB rejects under its default sql_mode (NO_ZERO_DATE,
	// NO_ZERO_IN_DATE): "keep" (default) leaves them and warns, "drop"
	// removes the DEFAULT clause.
	ZeroDateDefaults string `yaml:"zero_date_defaults"`

	// AutoRandom converts BIGINT AUTO_INCREMENT primary keys of the listed
	// tables to AUTO_RANDOM to avoid write hotspots. Opt-in.
	AutoRandom AutoRandomConfig `yaml:"auto_random"`

	// ShardRowIDBits adds SHARD_ROW_ID_BITS to tables without a clustered
	// primary key (no PRIMARY KEY, or NONCLUSTERED); 0 disables it.
	ShardRowIDBits uint64 `yaml:"shard_row_id_bits"`

	// PreSplitRegions adds PRE_SPLIT_REGIONS next to SHARD_ROW_ID_BITS; 0
	// leaves it out.
	PreSplitRegions uint64 `yaml:"pre_split_regions"`

	// Removed in v1.2.0 (never implemented); still accepted so existing
	// config files load. They have no effect.
	LegacyStrictMode       *bool `yaml:"strict_mode,omitempty"`
	LegacyPreserveComments *bool `yaml:"preserve_comments,omitempty"`
}

// AutoRandomConfig selects tables for AUTO_RANDOM conversion.
// https://docs.pingcap.com/tidb/stable/auto-random
type AutoRandomConfig struct {
	// Tables lists table names to convert, or "*" for every eligible table;
	// empty disables the conversion.
	Tables []string `yaml:"tables"`

	// ShardBits is AUTO_RANDOM's S (1-15).
	ShardBits int `yaml:"shard_bits"`

	// RangeBits is AUTO_RANDOM's R (32-64). The default 54 keeps every ID
	// below 2^53, so it stays exact in JavaScript and below PHP_INT_MAX; 64
	// is TiDB's own default and produces IDs up to 2^63-1.
	RangeBits int `yaml:"range_bits"`
}

// FormattingConfig controls SQL output formatting
type FormattingConfig struct {
	// IndentSize specifies the number of spaces for indentation
	IndentSize int `yaml:"indent_size"`

	// UppercaseKeywords determines if SQL keywords should be uppercase
	UppercaseKeywords bool `yaml:"uppercase_keywords"`

	// BackquoteIdentifiers determines if identifiers should be backtick-quoted.
	// Disabling it emits bare names, which breaks DDL for reserved words.
	BackquoteIdentifiers bool `yaml:"backquote_identifiers"`

	// SingleQuoteStrings determines if strings should use single quotes
	SingleQuoteStrings bool `yaml:"single_quote_strings"`
}

// CharsetMapping defines target charset and collation for a source charset
type CharsetMapping struct {
	TargetCharset   string `yaml:"target_charset"`
	TargetCollation string `yaml:"target_collation"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		InputDir:     "",
		OutputDir:    "",
		EnabledRules: []string{}, // Empty means all rules enabled
		// JsonGenerated is off by default: modern TiDB (v6.3+) supports JSON
		// functions in generated columns, so converting them to plain columns
		// is only needed for legacy targets.
		DisabledRules: []string{"JsonGenerated"},
		FormattingOptions: FormattingConfig{
			IndentSize:           2,
			UppercaseKeywords:    true,
			BackquoteIdentifiers: true,
			SingleQuoteStrings:   true,
		},
		CharsetMappings: map[string]CharsetMapping{
			"latin1": {
				TargetCharset:   "utf8mb4",
				TargetCollation: "utf8mb4_0900_ai_ci",
			},
		},
		CollationMappings: map[string]string{
			"latin1_swedish_ci": "utf8mb4_0900_ai_ci",
			"utf8mb4_unicode_*": "utf8mb4_0900_ai_ci",
		},
		ZeroDateDefaults: "keep",
		AutoRandom:       AutoRandomConfig{Tables: []string{}, ShardBits: 5, RangeBits: 54},
		AllowedDefaultFunctions: []string{
			"current_timestamp",
			"current_date",
			"current_time",
			"now",
			"localtime",
			"localtimestamp",
		},
	}
}

// IsRuleEnabled checks if a rule is enabled based on configuration
func (c *Config) IsRuleEnabled(ruleName string) bool {
	// If explicitly disabled, return false
	for _, disabled := range c.DisabledRules {
		if disabled == ruleName {
			return false
		}
	}

	// If EnabledRules is empty, all rules are enabled (except disabled ones)
	if len(c.EnabledRules) == 0 {
		return true
	}

	// If EnabledRules is specified, only those rules are enabled
	for _, enabled := range c.EnabledRules {
		if enabled == ruleName {
			return true
		}
	}

	return false
}

// LoadConfig reads configuration from a YAML file. If path is empty, returns
// DefaultConfig. Unknown keys and out-of-range values are errors: a typo
// would otherwise silently fall back to a default.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return DefaultConfig(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := DefaultConfig()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks values against what TiDB accepts.
func (c *Config) Validate() error {
	switch strings.ToLower(c.ZeroDateDefaults) {
	case "", "keep", "drop":
	default:
		return fmt.Errorf("zero_date_defaults must be keep or drop, got %q", c.ZeroDateDefaults)
	}
	ar := c.AutoRandom
	switch {
	case ar.ShardBits < 1 || ar.ShardBits > 15:
		return fmt.Errorf("auto_random.shard_bits must be 1-15, got %d", ar.ShardBits)
	case ar.RangeBits < 32 || ar.RangeBits > 64:
		return fmt.Errorf("auto_random.range_bits must be 32-64, got %d", ar.RangeBits)
	case ar.RangeBits-1-ar.ShardBits < 27:
		// https://docs.pingcap.com/tidb/stable/auto-random: at least 27
		// auto-increment bits remain after the sign and shard bits.
		return fmt.Errorf("auto_random: range_bits - shard_bits - 1 must be at least 27, got %d", ar.RangeBits-1-ar.ShardBits)
	}
	switch {
	case c.ShardRowIDBits > 15:
		return fmt.Errorf("shard_row_id_bits must be 0-15, got %d", c.ShardRowIDBits)
	case c.PreSplitRegions > c.ShardRowIDBits:
		return fmt.Errorf("pre_split_regions (%d) must not exceed shard_row_id_bits (%d)", c.PreSplitRegions, c.ShardRowIDBits)
	case c.FormattingOptions.IndentSize < 0:
		return fmt.Errorf("formatting.indent_size must not be negative")
	}
	return nil
}
