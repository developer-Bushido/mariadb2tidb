package parser

import (
	"strings"
	"testing"

	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regressions from an adversarial review: each input used to lose data or
// produce wrong SQL silently.

func mustParse(t *testing.T, sql string) []ast.StmtNode {
	t.Helper()
	stmts, err := NewLoader().ParseTiDB(sql)
	require.NoError(t, err, "output must parse:\n%s", sql)
	return stmts
}

func TestNoBackslashEscapesRoutineDoesNotSwallowTheDump(t *testing.T) {
	// mariadb-dump writes a routine created under NO_BACKSLASH_ESCAPES in
	// that mode; 'C:\' is a complete string there.
	dump := "CREATE TABLE before1 (a INT);\n" +
		"/*!50003 SET @saved_sql_mode       = @@sql_mode */ ;\n" +
		"/*!50003 SET sql_mode              = 'NO_BACKSLASH_ESCAPES,STRICT_TRANS_TABLES' */ ;\n" +
		"DELIMITER ;;\n" +
		"CREATE DEFINER=`root`@`%` PROCEDURE `p1`()\nSELECT 'C:\\' AS path ;;\n" +
		"DELIMITER ;\n" +
		"/*!50003 SET sql_mode              = @saved_sql_mode */ ;\n" +
		"CREATE TABLE after1 (p VARCHAR(10) DEFAULT 'C:\\\\x');\n" +
		"INSERT INTO after1 VALUES ('a\\'b');\n"
	got, warns := preprocessWith(t, dump)
	assert.Contains(t, got, "CREATE TABLE after1")
	assert.Contains(t, got, "INSERT INTO after1")
	assert.Equal(t, []string{"dropped PROCEDURE p1: TiDB does not support stored programs (triggers, procedures, functions, events)"}, warns)
	assert.Contains(t, got, "'STRICT_TRANS_TABLES'", "the output escapes backslashes, so the mode is not kept")
	stmts := mustParse(t, got)
	ins := stmts[len(stmts)-1].(*ast.InsertStmt)
	assert.Equal(t, "a'b", ins.Lists[0][0].(ast.ValueExpr).GetString())
}

func TestStatementsWrittenUnderNoBackslashEscapesKeepTheirValues(t *testing.T) {
	got, _ := preprocessWith(t, "SET sql_mode = 'NO_BACKSLASH_ESCAPES';\nINSERT INTO t VALUES ('C:\\new\\');\nSET sql_mode = DEFAULT;\nINSERT INTO t VALUES ('C:\\\\x');")
	stmts := mustParse(t, got)
	assert.Equal(t, `C:\new\`, stmts[1].(*ast.InsertStmt).Lists[0][0].(ast.ValueExpr).GetString())
	assert.Equal(t, `C:\x`, stmts[3].(*ast.InsertStmt).Lists[0][0].(ast.ValueExpr).GetString())
}

func TestCommentEndInsideExecutableCommentString(t *testing.T) {
	dump := "DELIMITER ;;\n" +
		"/*!50003 CREATE*/ /*!50003 TRIGGER trg BEFORE INSERT ON a FOR EACH ROW SET NEW.c = REPLACE(NEW.c, '*/', '') */;;\n" +
		"DELIMITER ;\n" +
		"CREATE TABLE b (id INT);\n" +
		"/*!50001 CREATE VIEW v AS select '*/' AS x, ';' AS y */;\n" +
		"CREATE TABLE c (id INT);\n"
	got, _ := preprocessWith(t, dump)
	stmts := mustParse(t, got)
	require.Len(t, stmts, 3)
	sel := stmts[1].(*ast.CreateViewStmt).Select.(*ast.SelectStmt)
	assert.Equal(t, ";", sel.Fields.Fields[1].Expr.(ast.ValueExpr).GetString())
}

func TestDelimiterTakesFirstWordOnly(t *testing.T) {
	got, _ := preprocessWith(t, "DELIMITER // -- routines\nCREATE PROCEDURE p() BEGIN SELECT 1; END //\nDELIMITER ;\nCREATE TABLE after1 (a INT);\nCREATE TABLE after2 (a INT);")
	assert.Len(t, mustParse(t, got), 2)
}

func TestUTF8BOM(t *testing.T) {
	got, _ := preprocessWith(t, "\ufeffDELIMITER ;;\nCREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW SET NEW.a = 1;;\nDELIMITER ;\nCREATE TABLE a (id INT);")
	assert.Len(t, mustParse(t, got), 1)
}

func TestSequenceEdgeCases(t *testing.T) {
	sql := "CREATE SEQUENCE d START WITH 3 MINVALUE 1 MAXVALUE 3 INCREMENT BY -1 NOCYCLE;\nDO SETVAL(`d`, 0, 0);\n" +
		"CREATE SEQUENCE n start with -10 minvalue -100 maxvalue -1 increment by 1;\nDO SETVAL(n, - 5, 0);\n" +
		"CREATE SEQUENCE plain INCREMENT BY 2;\nDO SETVAL(plain, 9, 0);\n" +
		"CREATE SEQUENCE c START WITH 1 MAXVALUE 5 CYCLE;\nDO SETVAL(c, 5, 1, 2);"
	got, warns := preprocessWith(t, sql)
	s := squash(got)
	assert.Contains(t, s, "START WITH 1 MINVALUE 1 MAXVALUE 3 INCREMENT BY -1 NOCYCLE; DO NEXTVAL(d);", "exhausted stays exhausted")
	assert.Contains(t, s, "start with -5 minvalue -100")
	assert.Contains(t, s, "plain START WITH 9 INCREMENT BY 2")
	assert.Contains(t, s, "c START WITH 1 MAXVALUE 5 CYCLE")
	assert.NotContains(t, s, "SETVAL")
	joined := strings.Join(warns, "\n")
	assert.Contains(t, joined, "sequence `d` is exhausted")
	assert.Contains(t, joined, "wrapped around")
	assert.Contains(t, joined, "round argument dropped")
	mustParse(t, got)
}

func TestColumnsNamedLikeKeywords(t *testing.T) {
	got, warns := preprocessWith(t, "CREATE TABLE t (id INT, period VARCHAR(10), charset VARCHAR(10), x INT) ENGINE=InnoDB, CHARSET=latin1;")
	s := squash(got)
	assert.Contains(t, s, "period VARCHAR(10)")
	assert.Contains(t, s, "charset VARCHAR(10)")
	assert.Contains(t, s, "CHARSET=utf8mb4 COLLATE utf8mb4_0900_ai_ci", "a comma-separated option is still an option")
	assert.Empty(t, warns)
	mustParse(t, got)
}

func TestDMLStringsAreData(t *testing.T) {
	sql := "UPDATE cfg SET charset = 'latin1' WHERE id=1;\nINSERT INTO cfg SET collation_server = 'latin1_swedish_ci';\nSELECT a COLLATE utf8mb4_uca1400_ai_ci FROM t;"
	got, _ := preprocessWith(t, sql)
	assert.Contains(t, got, "charset = 'latin1'")
	assert.Contains(t, got, "collation_server = 'latin1_swedish_ci'")
	assert.Contains(t, got, "a COLLATE utf8mb4_0900_ai_ci", "unknown collations must still become parseable")
	mustParse(t, got)
}

func TestMappedCharsetMovesItsCollation(t *testing.T) {
	got, _ := preprocessWith(t, "CREATE TABLE t (a VARCHAR(10) CHARACTER SET latin1 COLLATE latin1_bin) COLLATE=latin1_swedish_ci DEFAULT CHARSET=latin1;")
	s := squash(got)
	assert.Contains(t, s, "CHARACTER SET utf8mb4 COLLATE utf8mb4_bin")
	assert.Equal(t, 2, strings.Count(s, "COLLATE"), "no second COLLATE appended after an earlier one")
	mustParse(t, got)
}

func TestAlterTableForms(t *testing.T) {
	sql := "ALTER TABLE t ADD COLUMN IF NOT EXISTS u uuid, MODIFY COLUMN IF EXISTS g point, RENAME COLUMN encrypted TO enc, ALTER COLUMN transactional SET DEFAULT 1, ALGORITHM=INPLACE;"
	got, _ := preprocessWith(t, sql)
	s := squash(got)
	assert.Contains(t, s, "IF NOT EXISTS u char(36)")
	assert.Contains(t, s, "IF EXISTS g longblob")
	assert.Contains(t, s, "RENAME COLUMN encrypted TO enc")
	assert.Contains(t, s, "ALTER COLUMN transactional SET DEFAULT 1")
}

func TestMiscForms(t *testing.T) {
	got, warns := preprocessWith(t, "CREATE DEFINER=root@'%' TRIGGER trg BEFORE INSERT ON x FOR EACH ROW SET NEW.a = 1;\n"+
		"CREATE TABLE t2 (LIKE point);\nCREATE TABLE g (a INT, c INT AS (a+1) PERSISTENT);\n"+
		"CREATE TABLE s AS SELECT 1 AS encrypted;")
	s := squash(got)
	assert.NotContains(t, s, "TRIGGER")
	assert.Contains(t, s, "(LIKE point)")
	assert.Contains(t, s, "AS (a+1) STORED")
	assert.Contains(t, s, "SELECT 1 AS encrypted")
	assert.Len(t, warns, 2) // the trigger, and CREATE TABLE ... SELECT (unsupported by TiDB)
	mustParse(t, got)
}

// Second review round.

func TestCastCharsetIsAnExpression(t *testing.T) {
	// mariadb-dump writes views this way; appending COLLATE broke the file.
	sql := "CREATE TABLE t (a VARCHAR(5)) DEFAULT CHARSET=latin1;\n" +
		"/*!50001 CREATE ALGORITHM=UNDEFINED */ /*!50013 DEFINER=`root`@`%` SQL SECURITY DEFINER */ /*!50001 VIEW `v` AS select cast(`t`.`a` as char charset latin1) AS `c`, convert(`t`.`a` using latin1) AS `d` from `t` */;\n" +
		"CREATE TABLE g (a VARCHAR(5), b VARCHAR(5) AS (cast(a as char(5) charset latin1)) STORED);"
	got, _ := preprocessWith(t, sql)
	// Mapped like the columns (latin1 -> utf8mb4), and without a COLLATE,
	// which is not valid inside CAST.
	assert.Contains(t, got, "cast(`t`.`a` as char charset utf8mb4)")
	assert.Contains(t, got, "convert(`t`.`a` using utf8mb4)")
	assert.Contains(t, got, "cast(a as char(5) charset utf8mb4)")
	mustParse(t, got)
}

func TestViewStringsAreData(t *testing.T) {
	got, _ := preprocessWith(t, "CREATE VIEW v AS SELECT id FROM cfg WHERE charset = 'latin1' AND collation_server = 'x';")
	assert.Contains(t, got, "charset = 'latin1'")
	assert.Contains(t, got, "collation_server = 'x'")
}

func TestSqlModeExpressions(t *testing.T) {
	// REPLACE(...) contains the flag's name but turns it off.
	sql := "SET sql_mode = CONCAT(@@sql_mode, ',NO_BACKSLASH_ESCAPES');\n" +
		"INSERT INTO t VALUES ('C:\\');\n" +
		"SET sql_mode = REPLACE(@@sql_mode, 'NO_BACKSLASH_ESCAPES', '');\n" +
		"INSERT INTO t VALUES ('a\\\\b', 'line\\nbreak');\n" +
		"SET @m := @@SESSION.sql_mode, sql_mode = 'NO_BACKSLASH_ESCAPES';\n" +
		"INSERT INTO t VALUES ('x\\');\n" +
		"SET sql_mode = @m;\n" +
		"INSERT INTO t VALUES ('y\\\\');"
	got, _ := preprocessWith(t, sql)
	stmts := mustParse(t, got)
	val := func(i, j int) string { return stmts[i].(*ast.InsertStmt).Lists[0][j].(ast.ValueExpr).GetString() }
	assert.Equal(t, `C:\`, val(1, 0))
	assert.Equal(t, `a\b`, val(3, 0))
	assert.Equal(t, "line\nbreak", val(3, 1))
	assert.Equal(t, `x\`, val(5, 0))
	assert.Equal(t, `y\`, val(7, 0))
}

func TestQualifiedSetvalIsFolded(t *testing.T) {
	got, warns := preprocessWith(t, "CREATE SEQUENCE `db`.`s` start with -100 minvalue -1000 maxvalue -1 increment by 1;\nDO SETVAL(`db`.`s`, -50, 0);")
	assert.Contains(t, squash(got), "start with -50 ")
	assert.NotContains(t, got, "SETVAL")
	assert.Empty(t, warns)
}

func TestAlterAddColumnNamedCharset(t *testing.T) {
	got, warns := preprocessWith(t, "ALTER TABLE cfg ADD COLUMN charset varchar(10), ADD collation varchar(10);")
	assert.Empty(t, warns)
	mustParse(t, got)
}

// Fourth review round.

func TestSequencesInDifferentDatabases(t *testing.T) {
	sql := "CREATE SEQUENCE a.s START WITH 1 INCREMENT BY 1;\nCREATE SEQUENCE b.s START WITH 1 INCREMENT BY 1;\n" +
		"SELECT SETVAL(a.s, 1000, 1);\nSELECT SETVAL(b.s, 5, 1);\n" +
		"USE c; CREATE SEQUENCE s START WITH 1;\nUSE d; CREATE SEQUENCE s START WITH 1;\nUSE c; SELECT SETVAL(s, 70, 0);"
	got, _ := preprocessWith(t, sql)
	s := squash(got)
	assert.Contains(t, s, "a.s START WITH 1001")
	assert.Contains(t, s, "b.s START WITH 6")
	assert.Contains(t, s, "USE c; CREATE SEQUENCE s START WITH 70;")
	assert.Contains(t, s, "USE d; CREATE SEQUENCE s START WITH 1;")
	assert.NotContains(t, s, "SETVAL")
}

func TestCollateOnExplicitCharsetExpressions(t *testing.T) {
	sql := "CREATE VIEW v AS select convert(`a` using latin1) collate latin1_general_cs AS c, _latin1'x' collate latin1_general_ci AS d from t;\n" +
		"CREATE TABLE g (a VARCHAR(5), b VARCHAR(5) AS (cast(a as char charset latin1) collate latin1_bin) STORED, c VARCHAR(5) COLLATE latin1_general_cs) DEFAULT CHARSET=latin1;"
	got, warns := preprocessWith(t, sql+"\nCREATE VIEW w AS SELECT a FROM t WHERE a = _latin1'caf\xe9' COLLATE latin1_general_ci;")
	s := squash(got)
	// The expression follows the columns into utf8mb4, so comparing them
	// stays valid on TiDB (latin1 EXPLICIT vs utf8mb4 is error 1267).
	assert.Contains(t, s, "using utf8mb4) collate utf8mb4_bin AS c")
	assert.Contains(t, s, "_utf8mb4'x' collate utf8mb4_0900_ai_ci AS d")
	assert.Contains(t, s, "charset utf8mb4) collate utf8mb4_bin)")
	assert.Contains(t, s, "c VARCHAR(5) COLLATE utf8mb4_bin")
	// A non-ASCII literal cannot be re-labelled: it keeps latin1, and so
	// does its collation.
	assert.Contains(t, s, "_latin1'caf\xe9' COLLATE latin1_bin")
	assert.Contains(t, strings.Join(warns, "\n"), "keeps charset latin1")
	mustParse(t, got)
}

func TestSqlModeLikeMariaDB(t *testing.T) {
	cases := map[string]bool{ // script -> NO_BACKSLASH_ESCAPES after it
		"SET sql_mode='NO_BACKSLASH_ESCAPES'; SET sql_mode=REPLACE(@@sql_mode,'no_backslash_escapes','')": true, // REPLACE is case-sensitive
		"SET sql_mode='no_backslash_escapes'":                                                             true, // the mode itself is not
		"/*M!100000 SET sql_mode='NO_BACKSLASH_ESCAPES' */":                                               true,
		"SET sql_mode='NO_BACKSLASH_ESCAPES'; SET @x = @@sql_mode; SET sql_mode=''; SET sql_mode=@x":      true,
		"SET sql_mode='NO_BACKSLASH_ESCAPES'; SET sql_mode=REPLACE(@@sql_mode,'NO_BACKSLASH_ESCAPES','')": false,
	}
	cases["SET sql_mode=IF(1,'NO_BACKSLASH_ESCAPES','')"] = true
	cases["SET @m := 'NO_BACKSLASH_ESCAPES'; SET sql_mode = CONCAT_WS(',', @@sql_mode, @m)"] = true
	cases["SET sql_mode=UPPER('no_backslash_escapes')"] = true
	cases["/*!999999 SET sql_mode='NO_BACKSLASH_ESCAPES' */"] = false // never executed
	for script, want := range cases {
		st := splitScriptModes(script+";\nSELECT 1", nil)
		assert.Equal(t, want, st[len(st)-1].noBackslash, script)
	}
	var warned []string
	splitScriptModes("SET sql_mode = IF(@a = 1,'NO_BACKSLASH_ESCAPES',''); SET sql_mode = @undefined;", func(f string, a ...any) {
		warned = append(warned, f)
	})
	assert.Len(t, warned, 2, "unknown expressions and undefined variables are reported")
}

func TestCharsetComparisonsAndChange(t *testing.T) {
	got, warns := preprocessWith(t, "CREATE TABLE t (charset VARCHAR(10), x INT AS (charset = 'latin1') VIRTUAL);\nALTER TABLE t CHANGE x charset varchar(3);")
	assert.Contains(t, got, "charset = 'latin1'")
	assert.Empty(t, warns)
	mustParse(t, got)
}

func TestExecutableCommentVersions(t *testing.T) {
	got, _ := preprocessWith(t, "/*!999999 SET sql_mode='NO_BACKSLASH_ESCAPES' */;\n/*!100616 SET @x = 1 */;\nCREATE TABLE t (s VARCHAR(5));\nINSERT INTO t VALUES ('a\\\\b');")
	stmts := mustParse(t, got)
	assert.Equal(t, `a\b`, stmts[len(stmts)-1].(*ast.InsertStmt).Lists[0][0].(ast.ValueExpr).GetString())
	assert.Contains(t, got, "/*! SET @x = 1 */")
}
