// Package parser loads SQL text into TiDB parser ASTs (after token-level
// preprocessing of MariaDB-only syntax the parser rejects) and restores ASTs
// back into formatted SQL.
package parser

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver" // required: register TiDB SQL driver for parser
	"go.uber.org/zap"
)

// Loader handles loading and parsing SQL files
type Loader struct {
	parser *parser.Parser
	logger *zap.Logger
	pre    *preprocessor
}

// NewLoader creates a SQL loader without charset mappings; collations TiDB
// does not support still fall back to a supported one of the same charset.
func NewLoader() *Loader {
	return &Loader{
		parser: parser.New(),
		logger: utils.GetLogger(),
		pre:    &preprocessor{collations: compat.NewCollationResolver(nil), charsets: map[string]charsetTarget{}},
	}
}

// WithCharsetMappings configures the loader with charset and collation mappings
func (l *Loader) WithCharsetMappings(charsetMap map[string]string, collationMap map[string]string) *Loader {
	cfg := &config.Config{CollationMappings: collationMap}
	l.pre.collations = compat.NewCollationResolver(cfg)
	l.pre.charsets = map[string]charsetTarget{}
	for src, dst := range charsetMap {
		l.pre.charsets[strings.ToLower(src)] = charsetTarget{charset: dst}
	}
	return l
}

// NewLoaderWithConfig creates a loader configured with charset mappings from config
func NewLoaderWithConfig(cfg *config.Config) *Loader {
	loader := NewLoader()
	if cfg == nil {
		return loader
	}
	loader.pre.collations = compat.NewCollationResolver(cfg)
	for src, m := range cfg.CharsetMappings {
		loader.pre.charsets[strings.ToLower(src)] = charsetTarget{charset: m.TargetCharset, collation: m.TargetCollation}
	}
	return loader
}

// WithReporter makes the loader record migration warnings (dropped stored
// programs, stripped MariaDB-only attributes, ...) in r.
func (l *Loader) WithReporter(r *compat.Reporter) *Loader {
	l.pre.report = r
	return l
}

// LoadFromFile loads and parses SQL from a file
func (l *Loader) LoadFromFile(filename string) ([]ast.StmtNode, error) {
	l.logger.Info("Loading SQL file", zap.String("file", filename))

	file, err := os.Open(filename)
	if err != nil {
		l.logger.Debug("Failed to open file", zap.String("file", filename), zap.Error(err))
		return nil, err
	}
	defer func() { _ = file.Close() }()

	return l.LoadFromReader(file)
}

// LoadFromReader loads and parses SQL from an io.Reader
func (l *Loader) LoadFromReader(reader io.Reader) ([]ast.StmtNode, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		l.logger.Debug("Failed to read SQL content", zap.Error(err))
		return nil, err
	}

	return l.LoadFromString(string(content))
}

// LoadFromString loads and parses SQL from a string
func (l *Loader) LoadFromString(sql string) ([]ast.StmtNode, error) {
	l.logger.Debug("Parsing SQL", zap.Int("length", len(sql)))

	// Preprocess unsupported constructs before parsing
	sql = l.preprocessSQL(sql)

	stmts, _, err := l.parse(sql)
	if err != nil {
		l.logger.Debug("Failed to parse SQL", zap.Error(err))
		return nil, err
	}

	l.logger.Info("Successfully parsed SQL", zap.Int("statements", len(stmts)))
	return stmts, nil
}

// ParseTiDB parses SQL with the TiDB parser only, without preprocessing.
func (l *Loader) ParseTiDB(sql string) ([]ast.StmtNode, error) {
	stmts, _, err := l.parse(sql)
	return stmts, err
}

// preprocessSQL rewrites MariaDB-only syntax the TiDB parser rejects.
func (l *Loader) preprocessSQL(sql string) string {
	return l.pre.run(sql)
}

// ValidateFile parses a file with the TiDB parser without any preprocessing
// and returns the statement count, parser warnings, and the first hard error.
func (l *Loader) ValidateFile(filename string) (int, []error, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return 0, nil, err
	}
	return l.ValidateSQL(string(data))
}

// ValidateSQL parses SQL text with the TiDB parser without any preprocessing
// and returns the statement count, parser warnings, and the first hard error.
func (l *Loader) ValidateSQL(sql string) (int, []error, error) {
	stmts, warns, err := l.parse(sql)
	return len(stmts), warns, err
}

// parse runs the TiDB parser, converting its panics into errors: the
// lightweight test_driver panics on some literals (e.g. decimals longer than
// MyDecimal's 81 digits), and one bad value must not crash a whole run.
func (l *Loader) parse(sql string) (stmts []ast.StmtNode, warns []error, err error) {
	defer func() {
		if r := recover(); r != nil {
			stmts, warns, err = nil, nil, fmt.Errorf("TiDB parser panicked: %v", r)
		}
	}()
	return l.parser.Parse(sql, "", "")
}

// asciiLower lowercases ASCII letters only. Unlike strings.ToLower it never
// changes the byte length (invalid UTF-8, or letters such as U+0130), so
// offsets found in the result are valid in the input.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// isSpace reports whether b is an ASCII whitespace character.
func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

// isAlphaNum reports whether b is an ASCII letter or digit.
func isAlphaNum(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}
