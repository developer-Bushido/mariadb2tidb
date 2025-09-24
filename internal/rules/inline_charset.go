package rules

import (
	"regexp"
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/pingcap/tidb/pkg/parser/ast"
)

// InlineCharsetRule handles charset and collation transformations in column definitions
// that are specified inline (e.g., VARCHAR(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci)
// These are not handled properly by the standard CollationRule because TiDB parser
// doesn't expose them through the AST structure in a convenient way.
type InlineCharsetRule struct {
	charsetMap   map[string]config.CharsetMapping
	collationMap map[string]string
	// Regex patterns for matching inline charset/collation
	charsetRegex   *regexp.Regexp
	collationRegex *regexp.Regexp
}

// NewInlineCharsetRule creates a new InlineCharsetRule with the provided configuration
func NewInlineCharsetRule(cfg *config.Config) *InlineCharsetRule {
	rule := &InlineCharsetRule{
		charsetMap:   make(map[string]config.CharsetMapping),
		collationMap: make(map[string]string),
		// Match: CHARACTER SET charset_name
		charsetRegex: regexp.MustCompile(`(?i)character\s+set\s+([a-zA-Z0-9_]+)`),
		// Match: COLLATE collation_name
		collationRegex: regexp.MustCompile(`(?i)collate\s+([a-zA-Z0-9_]+)`),
	}

	if cfg != nil {
		for k, v := range cfg.CharsetMappings {
			rule.charsetMap[strings.ToLower(k)] = v
		}
		for k, v := range cfg.CollationMappings {
			rule.collationMap[strings.ToLower(k)] = v
		}
	}

	return rule
}

// Name returns the unique name of the rule
func (r *InlineCharsetRule) Name() string {
	return "InlineCharset"
}

// Description returns a human-readable description of what the rule does
func (r *InlineCharsetRule) Description() string {
	return "Transform inline CHARACTER SET and COLLATE specifications in column definitions"
}

// Priority returns the priority of the rule (should run before CollationRule)
func (r *InlineCharsetRule) Priority() int {
	return 50 // Higher priority than CollationRule (100)
}

// ShouldApply checks if the rule should be applied to the given AST node
func (r *InlineCharsetRule) ShouldApply(node ast.Node) bool {
	// Only apply to column definitions
	if _, ok := node.(*ast.ColumnDef); !ok {
		return false
	}

	// Check if the node text contains inline charset or collation
	nodeText := node.Text()
	return r.charsetRegex.MatchString(nodeText) || r.collationRegex.MatchString(nodeText)
}

// Apply applies the transformation to the AST node
// Note: This is a bit of a hack - we modify the node's internal representation
// by working with its text representation, but it's the most reliable way
// to handle inline charset/collation specifications that TiDB parser doesn't
// expose properly through the AST structure.
func (r *InlineCharsetRule) Apply(node ast.Node) (ast.Node, error) {
	col, ok := node.(*ast.ColumnDef)
	if !ok {
		return node, nil
	}

	// This is where it gets tricky - we need to modify the column definition
	// but the TiDB AST doesn't give us direct access to charset/collation in FieldType
	//
	// For now, we'll work with the FieldType's Charset and Collate fields if available
	// and fall back to text manipulation if needed

	// Try to access charset through FieldType
	if col.Tp != nil {
		// Check and transform charset
		if charset := col.Tp.GetCharset(); charset != "" {
			if mapping, ok := r.charsetMap[strings.ToLower(charset)]; ok {
				col.Tp.SetCharset(mapping.TargetCharset)
				if mapping.TargetCollation != "" {
					col.Tp.SetCollate(mapping.TargetCollation)
				}
			}
		}

		// Check and transform collation
		if collation := col.Tp.GetCollate(); collation != "" {
			if newCollation, ok := r.collationMap[strings.ToLower(collation)]; ok {
				col.Tp.SetCollate(newCollation)
			}
		}
	}

	return col, nil
}
