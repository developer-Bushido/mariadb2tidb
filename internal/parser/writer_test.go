package parser

import (
	"strings"
	"testing"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	tidbparser "github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterFormatsCreateTable(t *testing.T) {
	// Initialize logger for tests
	require.NoError(t, initTestLogger())

	loader := NewLoader()
	writer := NewWriter()

	sql := `CREATE TABLE users (
		id int(11) NOT NULL AUTO_INCREMENT,
		name varchar(255) NOT NULL,
		PRIMARY KEY (id)
	) ENGINE=InnoDB;`

	// Parse the SQL
	stmts, err := loader.LoadFromString(sql)
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	// Write it back
	result, err := writer.WriteStatement(stmts[0])
	require.NoError(t, err)

	// Check that the result is valid SQL and properly formatted
	upper := strings.ToUpper(result)
	assert.Contains(t, upper, "CREATE TABLE")
	assert.Contains(t, upper, "USERS")
	assert.Contains(t, upper, "PRIMARY KEY")
	assert.True(t, strings.HasSuffix(strings.TrimSpace(result), ";"))
}

func TestParserRoundTrip(t *testing.T) {
	// Initialize logger for tests
	require.NoError(t, initTestLogger())

	loader := NewLoader()
	writer := NewWriter()

	sql := `CREATE TABLE test (id INT PRIMARY KEY);`

	// Parse the SQL
	stmts, err := loader.LoadFromString(sql)
	require.NoError(t, err)
	require.Len(t, stmts, 1)

	// Write it back
	result, err := writer.WriteStatement(stmts[0])
	require.NoError(t, err)

	// Parse the result again to ensure it's valid
	stmts2, err := loader.LoadFromString(result)
	require.NoError(t, err)
	require.Len(t, stmts2, 1)

	// The statements should be equivalent (though formatting may differ)
	upper := strings.ToUpper(result)
	assert.Contains(t, upper, "CREATE TABLE")
	assert.Contains(t, upper, "TEST")
}

func TestWriteStatements(t *testing.T) {
	// Initialize logger for tests
	require.NoError(t, initTestLogger())

	loader := NewLoader()
	writer := NewWriter()

	sql := `CREATE TABLE test1 (id INT);
CREATE TABLE test2 (name VARCHAR(50));`

	// Parse the SQL
	stmts, err := loader.LoadFromString(sql)
	require.NoError(t, err)
	require.Len(t, stmts, 2)

	// Write all statements
	result, err := writer.WriteStatements(stmts)
	require.NoError(t, err)

	// Check that both statements are present
	assert.Contains(t, result, "test1")
	assert.Contains(t, result, "test2")

	// Check that statements are separated
	lines := strings.Split(result, "\n")
	assert.True(t, len(lines) > 2)
}

// TestWriterQuotesReservedIdentifiers guards against the formatter dropping
// backticks: every TiDB reserved word used as a table, column and index name
// must survive the round trip and re-parse with the TiDB parser.
func TestWriterQuotesReservedIdentifiers(t *testing.T) {
	require.NoError(t, initTestLogger())

	loader := NewLoader()
	writer := NewWriter()

	for _, kw := range tidbparser.Keywords {
		if !kw.Reserved {
			continue
		}
		name := strings.ToLower(kw.Word)
		t.Run(name, func(t *testing.T) {
			sql := "CREATE TABLE `" + name + "` (`" + name + "` INT, KEY `" + name + "` (`" + name + "`));"
			stmts, err := loader.LoadFromString(sql)
			require.NoError(t, err)

			got, err := writer.WriteStatement(stmts[0])
			require.NoError(t, err)

			_, warns, err := loader.ValidateSQL(got)
			require.NoError(t, err, "output must re-parse:\n%s", got)
			require.Empty(t, warns)
		})
	}
}

// TestWriterKeepsTiDBSpecificDefinitions guards against formatter fallbacks
// that silently drop column definitions for syntax a MySQL-only parser does
// not know.
func TestWriterKeepsTiDBSpecificDefinitions(t *testing.T) {
	require.NoError(t, initTestLogger())

	sql := "CREATE TABLE a (id BIGINT AUTO_RANDOM, v INT, PRIMARY KEY (id) CLUSTERED) SHARD_ROW_ID_BITS=4;"
	stmts, err := NewLoader().LoadFromString(sql)
	require.NoError(t, err)

	got, err := NewWriter().WriteStatement(stmts[0])
	require.NoError(t, err)

	assert.Equal(t, "CREATE TABLE `a` (\n"+
		"  `id` BIGINT AUTO_RANDOM,\n"+
		"  `v` INT,\n"+
		"  PRIMARY KEY(`id`) CLUSTERED\n"+
		") SHARD_ROW_ID_BITS = 4;", got)
}

func TestWriterHonorsFormattingConfig(t *testing.T) {
	require.NoError(t, initTestLogger())

	cfg := config.DefaultConfig()
	cfg.FormattingOptions = config.FormattingConfig{
		IndentSize:           4,
		UppercaseKeywords:    false,
		BackquoteIdentifiers: false,
		SingleQuoteStrings:   true,
	}

	stmts, err := NewLoader().LoadFromString("CREATE TABLE t (id INT NOT NULL DEFAULT 1, s VARCHAR(5) DEFAULT 'x');")
	require.NoError(t, err)

	got, err := NewWriterWithConfig(cfg).WriteStatement(stmts[0])
	require.NoError(t, err)

	assert.Equal(t, "create table t (\n"+
		"    id int not null default 1,\n"+
		"    s varchar(5) default _utf8mb4'x'\n"+
		");", got)
}

// TestWriterRoundTripIsStable checks that output re-parses with the TiDB
// parser and is written back byte-for-byte identically, i.e. the writer
// preserves meaning (escapes, names, options) instead of only looking right.
func TestWriterRoundTripIsStable(t *testing.T) {
	require.NoError(t, initTestLogger())

	cases := map[string]string{
		"backslash default":   `CREATE TABLE t (p VARCHAR(64) DEFAULT 'C:\\new', e CHAR(1) NOT NULL DEFAULT '\\');`,
		"backslash comment":   `CREATE TABLE t (id INT COMMENT 'regex \\d+') COMMENT='x\\';`,
		"backslash insert":    `INSERT INTO t VALUES ('C:\\x\\', 'y');`,
		"quotes":              `CREATE TABLE t (s VARCHAR(10) DEFAULT 'it''s "q"');`,
		"charset introducers": `CREATE TABLE t (a VARCHAR(5) CHARACTER SET latin1 DEFAULT _latin1'x', b VARBINARY(3) DEFAULT _binary'abc');`,
		"name like body":      "CREATE TABLE ` (```` INT)` (```` INT);",
		"comment like body":   "CREATE TABLE t (id INT) COMMENT=' (`id` INT)';",
		"global temporary":    "CREATE GLOBAL TEMPORARY TABLE t (id INT) ON COMMIT DELETE ROWS;",
		"if not exists":       "CREATE TABLE IF NOT EXISTS d.t (id INT, PRIMARY KEY (id));",
		"like":                "CREATE TABLE t2 LIKE t1;",
		"as select":           "CREATE TABLE t (id INT) AS SELECT 1 AS id;",
		"generated and check": "CREATE TABLE t (a INT, b INT AS (a + 1) STORED, CONSTRAINT c CHECK (a > 0));",
		"foreign key":         "CREATE TABLE t (a INT, FOREIGN KEY (a) REFERENCES p (id) ON DELETE CASCADE);",
		"partitioned":         "CREATE TABLE t (id INT, d DATE) PARTITION BY RANGE COLUMNS (d) (PARTITION p0 VALUES LESS THAN ('2020-01-01'), PARTITION p1 VALUES LESS THAN (MAXVALUE));",
		"alter":               `ALTER TABLE t ADD COLUMN c VARCHAR(5) DEFAULT 'a\\b', ADD INDEX i (c);`,
	}

	loader := NewLoader()
	writer := NewWriter()
	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			stmts, err := loader.LoadFromString(sql)
			require.NoError(t, err)
			first, err := writer.WriteStatement(stmts[0])
			require.NoError(t, err)

			again, err := loader.LoadFromString(first)
			require.NoError(t, err, "output must re-parse:\n%s", first)
			second, err := writer.WriteStatement(again[0])
			require.NoError(t, err)
			assert.Equal(t, first, second)

			// Layout aside, the re-parsed statement must equal the input.
			assert.Equal(t, canonical(t, stmts[0]), canonical(t, again[0]))
		})
	}
}

// canonical restores a node on one line with fixed flags, for comparing ASTs.
func canonical(t *testing.T, n ast.Node) string {
	t.Helper()
	var sb strings.Builder
	flags := format.DefaultRestoreFlags | format.RestoreStringEscapeBackslash
	require.NoError(t, n.Restore(format.NewRestoreCtx(flags, &sb)))
	return sb.String()
}

func TestWriterEscapesBackslashes(t *testing.T) {
	require.NoError(t, initTestLogger())

	stmts, err := NewLoader().LoadFromString(`CREATE TABLE t (p VARCHAR(64) DEFAULT 'C:\\new');`)
	require.NoError(t, err)
	got, err := NewWriter().WriteStatement(stmts[0])
	require.NoError(t, err)

	reparsed, err := NewLoader().LoadFromString(got)
	require.NoError(t, err)
	def := reparsed[0].(*ast.CreateTableStmt).Cols[0].Options[0].Expr.(ast.ValueExpr)
	assert.Equal(t, `C:\new`, def.GetString())
}

// TestWriterKeepsCharsetIntroducers: without the introducer a literal's bytes
// would be reinterpreted under the importing session's SET NAMES.
func TestWriterKeepsCharsetIntroducers(t *testing.T) {
	require.NoError(t, initTestLogger())

	stmts, err := NewLoader().LoadFromString(`INSERT INTO t VALUES (_utf8mb4'café', _latin1'x', '');`)
	require.NoError(t, err)
	got, err := NewWriter().WriteStatement(stmts[0])
	require.NoError(t, err)

	assert.Equal(t, "INSERT INTO `t` VALUES (_UTF8MB4'café',_LATIN1'x','');", got)
}

// TestWriterEscapesNUL: a raw NUL byte in the output is rejected by the mysql
// client outside --binary-mode.
func TestWriterEscapesNUL(t *testing.T) {
	require.NoError(t, initTestLogger())

	stmts, err := NewLoader().LoadFromString(`INSERT INTO t VALUES ('a\0b');`)
	require.NoError(t, err)
	got, err := NewWriter().WriteStatement(stmts[0])
	require.NoError(t, err)
	assert.NotContains(t, got, "\x00")

	reparsed, err := NewLoader().LoadFromString(got)
	require.NoError(t, err)
	v := reparsed[0].(*ast.InsertStmt).Lists[0][0].(ast.ValueExpr)
	assert.Equal(t, "a\x00b", v.GetString())
}

func TestNewWriterWithNilConfig(t *testing.T) {
	require.NoError(t, initTestLogger())

	stmts, err := NewLoader().LoadFromString("CREATE TABLE t (id INT);")
	require.NoError(t, err)
	got, err := NewWriterWithConfig(nil).WriteStatement(stmts[0])
	require.NoError(t, err)
	assert.Equal(t, "CREATE TABLE `t` (\n  `id` INT\n);", got)
}
