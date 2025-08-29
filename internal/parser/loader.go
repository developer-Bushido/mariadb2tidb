package parser

import (
    "io"
    "os"
    "regexp"
    "strings"

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
}

// NewLoader creates a new SQL loader
func NewLoader() *Loader {
	return &Loader{
		parser: parser.New(),
		logger: utils.GetLogger(),
	}
}

// LoadFromFile loads and parses SQL from a file
func (l *Loader) LoadFromFile(filename string) ([]ast.StmtNode, error) {
	l.logger.Info("Loading SQL file", zap.String("file", filename))

	file, err := os.Open(filename)
	if err != nil {
		l.logger.Error("Failed to open file", zap.String("file", filename), zap.Error(err))
		return nil, err
	}
	defer file.Close()

	return l.LoadFromReader(file)
}

// LoadFromReader loads and parses SQL from an io.Reader
func (l *Loader) LoadFromReader(reader io.Reader) ([]ast.StmtNode, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		l.logger.Error("Failed to read SQL content", zap.Error(err))
		return nil, err
	}

	return l.LoadFromString(string(content))
}

// LoadFromString loads and parses SQL from a string
func (l *Loader) LoadFromString(sql string) ([]ast.StmtNode, error) {
	l.logger.Debug("Parsing SQL", zap.Int("length", len(sql)))

	// Preprocess unsupported constructs before parsing
	sql = preprocessSQL(sql)

	stmts, _, err := l.parser.Parse(sql, "", "")
	if err != nil {
		l.logger.Error("Failed to parse SQL", zap.Error(err))
		return nil, err
	}

	l.logger.Info("Successfully parsed SQL", zap.Int("statements", len(stmts)))
	return stmts, nil
}

var (
	uuidRegex      = regexp.MustCompile(`(?i)\buuid\b`)
	encryptedRegex = regexp.MustCompile("(?i)\\s*`encrypted`\\s*=\\s*yes\\s*`encryption_key_id`\\s*=\\s*\\d+\\s*")
	char36Regex    = regexp.MustCompile(`(?i)char\(36\)`)
	uuidKeyRegex   = regexp.MustCompile("(?i)unique\\s+key\\s+`?uuid`?")
)

// preprocessSQL performs lightweight text-based transformations before AST parsing.
// Currently handles UUID type replacement to ensure TiDB parser compatibility.
func preprocessSQL(sql string) string {
	// Remove MariaDB table encryption options that TiDB doesn't support
	sql = encryptedRegex.ReplaceAllString(sql, " ")

	// Replace standalone UUID data types with char(36) but keep functions and identifiers
	matches := uuidRegex.FindAllStringIndex(sql, -1)
	if matches == nil {
		return sql
	}

	var result strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		result.WriteString(sql[last:start])

        // Find preceding non-space/non-backtick character
        j := start - 1
        for j >= 0 && (isSpace(sql[j]) || sql[j] == '`') {
            j--
        }

        // Find following non-space/non-backtick character
        i := end
        for i < len(sql) && (isSpace(sql[i]) || sql[i] == '`') {
            i++
        }

        // Extract preceding word for context checks
        wordEnd := j
        for wordEnd >= 0 && (isAlphaNum(sql[wordEnd]) || sql[wordEnd] == '_') {
            wordEnd--
        }
        precedingWord := strings.ToLower(sql[wordEnd+1 : j+1])

		switch {
		case i < len(sql) && sql[i] == '(':
			// uuid used as function - leave unchanged
			result.WriteString(sql[start:end])
		case i < len(sql) && sql[i] == '`':
			// uuid inside backticks - identifier
			result.WriteString(sql[start:end])
		case precedingWord == "key" || precedingWord == "unique" || precedingWord == "primary" || precedingWord == "constraint" || precedingWord == "index":
			// index or constraint name - leave unchanged
			result.WriteString(sql[start:end])
        case j >= 0 && (isAlphaNum(sql[j]) || sql[j] == '_'):
            // uuid used as a data type - replace
            result.WriteString("char(36)")
        default:
            // uuid as column name or other - leave unchanged
            result.WriteString(sql[start:end])
        }

		last = end
	}

	result.WriteString(sql[last:])

	processed := result.String()

	// Rename unique key named "uuid" to "uuid_key"
	processed = uuidKeyRegex.ReplaceAllString(processed, "UNIQUE KEY uuid_key")

	// Add default '' to char(36) NOT NULL columns missing an explicit default
	matches = char36Regex.FindAllStringIndex(processed, -1)
	if matches == nil {
		return processed
	}

	var out strings.Builder
	last = 0
	for _, m := range matches {
		end := m[1]
		out.WriteString(processed[last:end])

		rest := processed[end:]
		segEnd := strings.IndexAny(rest, ",)")
		if segEnd == -1 {
			segEnd = len(rest)
		}
		segment := rest[:segEnd]
		lowerSeg := strings.ToLower(segment)
		if strings.Contains(lowerSeg, "not null") && !strings.Contains(lowerSeg, "default") {
			idx := strings.Index(lowerSeg, "not null") + len("not null")
			segment = segment[:idx] + " default ''" + segment[idx:]
		}
		out.WriteString(segment)
		last = end + segEnd
	}
	out.WriteString(processed[last:])
	return out.String()
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
