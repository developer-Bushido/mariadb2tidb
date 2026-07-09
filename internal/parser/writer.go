package parser

import (
	"regexp"
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/format"
	"go.uber.org/zap"
	sqlparser "vitess.io/vitess/go/vt/sqlparser"
)

// charsetEmptyDefaultRegex matches charset-prefixed empty string defaults
// (e.g. DEFAULT _utf8mb4 ”) that the restore step emits.
var charsetEmptyDefaultRegex = regexp.MustCompile(`(?i)DEFAULT\s+_utf8mb4\s+''`)

// Writer handles writing AST back to formatted SQL
type Writer struct {
	logger *zap.Logger
}

// NewWriter creates a new SQL writer
func NewWriter() *Writer {
	return &Writer{
		logger: utils.GetLogger(),
	}
}

// WriteStatements writes multiple statements to formatted SQL
func (w *Writer) WriteStatements(stmts []ast.StmtNode) (string, error) {
	w.logger.Debug("Writing statements to SQL", zap.Int("count", len(stmts)))

	var result strings.Builder

	for i, stmt := range stmts {
		if i > 0 {
			result.WriteString("\n\n")
		}

		formatted, err := w.WriteStatement(stmt)
		if err != nil {
			w.logger.Error("Failed to write statement", zap.Int("index", i), zap.Error(err))
			return "", err
		}

		result.WriteString(formatted)
	}

	return result.String(), nil
}

// WriteStatement writes a single statement to formatted SQL
func (w *Writer) WriteStatement(stmt ast.StmtNode) (string, error) {
	w.logger.Debug("Writing statement to SQL", zap.String("type", stmt.Text()))

	var sb strings.Builder

	// Configure restore flags for readable output
	flags := format.DefaultRestoreFlags

	ctx := format.NewRestoreCtx(flags, &sb)

	err := stmt.Restore(ctx)
	if err != nil {
		w.logger.Error("Failed to restore statement", zap.Error(err))
		return "", err
	}

	result := sb.String()

	// Use vitess parser to pretty format the SQL
	if formatted, err := w.formatSQL(result); err == nil {
		result = formatted
	} else {
		w.logger.Debug("SQL formatting failed", zap.Error(err))
	}

	// Remove charset prefixes on empty string defaults
	result = charsetEmptyDefaultRegex.ReplaceAllString(result, "DEFAULT ''")

	// Ensure statement ends with semicolon
	if !strings.HasSuffix(result, ";") {
		result += ";"
	}

	return result, nil
}

// formatSQL formats SQL using vitess sqlparser
func (w *Writer) formatSQL(sql string) (string, error) {
	p, err := sqlparser.New(sqlparser.Options{})
	if err != nil {
		return "", err
	}
	stmt, err := p.Parse(sql)
	if err != nil {
		return "", err
	}
	return sqlparser.String(stmt), nil
}
