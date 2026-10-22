package parser

import (
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/format"
	"go.uber.org/zap"
)

// defaultIndentSize is used when formatting.indent_size is not positive.
const defaultIndentSize = 2

// Writer handles writing AST back to formatted SQL. Output is produced by the
// TiDB parser's own Restore, so keywords and identifier quoting always follow
// the TiDB grammar that validated the input.
type Writer struct {
	logger *zap.Logger
	flags  format.RestoreFlags
	indent string
}

// NewWriter creates a new SQL writer with default formatting options.
func NewWriter() *Writer {
	return NewWriterWithConfig(config.DefaultConfig())
}

// NewWriterWithConfig creates a new SQL writer honoring cfg.FormattingOptions.
func NewWriterWithConfig(cfg *config.Config) *Writer {
	opts := cfg.FormattingOptions

	// String literals in the default charset (utf8mb4) are written without a
	// redundant _utf8mb4 introducer; other introducers carry meaning and stay.
	// Backslashes must be escaped or 'C:\\new' would load as "C:<LF>ew".
	flags := format.RestoreStringWithoutDefaultCharset | format.RestoreStringEscapeBackslash
	if opts.SingleQuoteStrings {
		flags |= format.RestoreStringSingleQuotes
	} else {
		flags |= format.RestoreStringDoubleQuotes
	}
	if opts.UppercaseKeywords {
		flags |= format.RestoreKeyWordUppercase
	} else {
		flags |= format.RestoreKeyWordLowercase
	}
	if opts.BackquoteIdentifiers {
		flags |= format.RestoreNameBackQuotes
	}

	size := opts.IndentSize
	if size <= 0 {
		size = defaultIndentSize
	}

	return &Writer{
		logger: utils.GetLogger(),
		flags:  flags,
		indent: strings.Repeat(" ", size),
	}
}

// WriteStatements writes multiple statements to formatted SQL, separated by
// a blank line and terminated by a newline.
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
	if len(stmts) > 0 {
		result.WriteString("\n")
	}

	return result.String(), nil
}

// WriteStatement writes a single statement to formatted SQL
func (w *Writer) WriteStatement(stmt ast.StmtNode) (string, error) {
	w.logger.Debug("Writing statement to SQL", zap.String("type", stmt.Text()))

	result, err := w.restore(stmt)
	if err != nil {
		w.logger.Error("Failed to restore statement", zap.Error(err))
		return "", err
	}

	if ct, ok := stmt.(*ast.CreateTableStmt); ok {
		if pretty, err := w.prettyCreateTable(ct, result); err == nil {
			result = pretty
		} else {
			w.logger.Debug("CREATE TABLE pretty-printing failed", zap.Error(err))
		}
	}

	// Ensure statement ends with semicolon
	if !strings.HasSuffix(result, ";") {
		result += ";"
	}

	return result, nil
}

func (w *Writer) restore(node ast.Node) (string, error) {
	var sb strings.Builder
	if err := node.Restore(format.NewRestoreCtx(w.flags, &sb)); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// prettyCreateTable puts each column and constraint of a restored CREATE
// TABLE on its own line. Restore writes the definitions as " (" + defs
// joined by "," + ")" right after the table name, so the parts are restored
// individually and that exact span, at that exact offset, is replaced; the
// output stays byte-for-byte TiDB's apart from layout.
func (w *Writer) prettyCreateTable(ct *ast.CreateTableStmt, single string) (string, error) {
	defs := make([]string, 0, len(ct.Cols)+len(ct.Constraints))
	for _, col := range ct.Cols {
		s, err := w.restore(col)
		if err != nil {
			return "", err
		}
		defs = append(defs, s)
	}
	for _, c := range ct.Constraints {
		s, err := w.restore(c)
		if err != nil {
			return "", err
		}
		defs = append(defs, s)
	}
	if len(defs) == 0 {
		return single, nil
	}

	// The head is everything before the definitions. Restoring a copy without
	// them yields the head, plus " ON COMMIT ..." for global temporary tables.
	bare := *ct
	bare.Cols, bare.Constraints, bare.Options, bare.Partition, bare.Select = nil, nil, nil, nil, nil
	head, err := w.restore(&bare)
	if err != nil {
		return "", err
	}
	if ct.TemporaryKeyword == ast.TemporaryGlobal {
		if j := strings.LastIndex(asciiLower(head), " on commit "); j >= 0 {
			head = head[:j]
		}
	}

	span := " (" + strings.Join(defs, ",") + ")"
	if !strings.HasPrefix(single, head) || !strings.HasPrefix(single[len(head):], span) {
		return single, nil
	}
	body := " (\n" + w.indent + strings.Join(defs, ",\n"+w.indent) + "\n)"
	return head + body + single[len(head)+len(span):], nil
}
