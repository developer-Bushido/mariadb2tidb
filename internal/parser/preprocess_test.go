package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
)

func preprocessWith(t *testing.T, sql string) (string, []string) {
	t.Helper()
	rep := compat.NewReporter()
	l := NewLoaderWithConfig(config.DefaultConfig()).WithReporter(rep)
	return l.preprocessSQL(sql), rep.Warnings()
}

// squash collapses whitespace so assertions do not depend on layout.
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestPreprocessLeavesStringsAndCommentsAlone(t *testing.T) {
	sql := "CREATE TABLE t (id INT, note VARCHAR(50) COMMENT 'a uuid value, latin1_swedish_ci');\n" +
		"INSERT INTO t VALUES (1,'generate a uuid here'),(2,'x char(36) not null y');"
	got, _ := preprocessWith(t, sql)
	assert.Contains(t, got, "'a uuid value, latin1_swedish_ci'")
	assert.Contains(t, got, "'generate a uuid here'")
	assert.Contains(t, got, "'x char(36) not null y'")
}

func TestPreprocessColumnTypes(t *testing.T) {
	got, warns := preprocessWith(t, "CREATE TABLE t (`uuid` uuid NOT NULL, ip4 INET4, ip6 inet6 DEFAULT '::1', g POINT, d DEFAULT uuid(), UNIQUE KEY uuid (`uuid`), SPATIAL KEY sg (g))")
	s := squash(got)
	assert.Contains(t, s, "`uuid` char(36) NOT NULL DEFAULT ''")
	assert.Contains(t, s, "ip4 varchar(15)")
	assert.Contains(t, s, "ip6 varchar(39) DEFAULT '::1'")
	assert.Contains(t, s, "g longblob")
	assert.Contains(t, s, "DEFAULT uuid()", "a uuid() call is not a type")
	assert.Contains(t, s, "UNIQUE KEY uuid (`uuid`)", "index names are not types")
	assert.NotContains(t, s, "SPATIAL")
	assert.Len(t, warns, 2) // POINT -> LONGBLOB, SPATIAL index dropped
}

func TestPreprocessMariaDBAttributesAndOptions(t *testing.T) {
	sql := "CREATE TABLE t (id INT PRIMARY KEY, secret INT INVISIBLE, b BLOB COMPRESSED, v VARCHAR(10) COMPRESSED=zlib, " +
		"rs TIMESTAMP(6) GENERATED ALWAYS AS ROW START, x INT WITHOUT SYSTEM VERSIONING, KEY k (id) INVISIBLE, FULLTEXT KEY ft (v)) " +
		"ENGINE=InnoDB `PAGE_COMPRESSED`='1' PAGE_COMPRESSION_LEVEL=6 TRANSACTIONAL=1 `encrypted`=yes `encryption_key_id`=2 WITH SYSTEM VERSIONING"
	got, warns := preprocessWith(t, sql)
	s := squash(got)
	for _, gone := range []string{"secret INT INVISIBLE", "COMPRESSED", "zlib", "GENERATED ALWAYS AS ROW", "SYSTEM VERSIONING", "PAGE_", "TRANSACTIONAL", "encrypt"} {
		assert.NotContains(t, s, gone)
	}
	assert.Contains(t, s, "KEY k (id) INVISIBLE", "invisible indexes are supported by TiDB")
	assert.Contains(t, s, "ENGINE=InnoDB")
	assert.GreaterOrEqual(t, len(warns), 5)

	_, err := NewLoader().ParseTiDB(got)
	require.NoError(t, err, "preprocessed DDL must parse:\n%s", got)
}

func TestPreprocessDropsStoredPrograms(t *testing.T) {
	dump := "/*M!999999\\- enable the sandbox mode */\n" +
		"CREATE TABLE t (a INT);\n" +
		"DELIMITER ;;\n" +
		"/*!50003 CREATE*/ /*!50017 DEFINER=`root`@`%`*/ /*!50003 TRIGGER trg BEFORE INSERT ON t FOR EACH ROW BEGIN SET NEW.a = 1; END */;;\n" +
		"CREATE DEFINER=`root`@`%` PROCEDURE `p1`()\nBEGIN SELECT 1; END ;;\n" +
		"CREATE DEFINER=CURRENT_USER() FUNCTION f1(x INT) RETURNS INT DETERMINISTIC RETURN x + 1 ;;\n" +
		"DELIMITER ;\n" +
		"/*!50106 DROP EVENT IF EXISTS `ev1` */;\n" +
		"/*!50003 DROP PROCEDURE IF EXISTS `p1` */;\n" +
		"CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`%` SQL SECURITY DEFINER VIEW v AS SELECT a FROM t;"
	got, warns := preprocessWith(t, dump)
	assert.NotContains(t, got, "TRIGGER")
	assert.NotContains(t, got, "PROCEDURE")
	assert.NotContains(t, got, "FUNCTION")
	assert.NotContains(t, got, "sandbox")
	assert.Contains(t, got, "VIEW v AS SELECT a FROM t")
	assert.ElementsMatch(t, []string{
		"dropped TRIGGER trg: TiDB does not support stored programs (triggers, procedures, functions, events)",
		"dropped PROCEDURE p1: TiDB does not support stored programs (triggers, procedures, functions, events)",
		"dropped FUNCTION f1: TiDB does not support stored programs (triggers, procedures, functions, events)",
	}, warns)

	stmts, err := NewLoader().ParseTiDB(got)
	require.NoError(t, err)
	assert.Len(t, stmts, 2)
}

func TestPreprocessCollations(t *testing.T) {
	sql := "CREATE DATABASE d /*!40100 DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_uca1400_ai_ci */;\n" +
		"SET collation_connection = latin1_swedish_ci;\n" +
		"SET @@SESSION.collation_connection = utf8mb3_uca1400_as_cs;\n" +
		"CREATE TABLE t (a VARCHAR(5) COLLATE utf8mb4_uca1400_as_ci, b VARCHAR(5) CHARACTER SET latin1) DEFAULT CHARSET=latin1 COLLATE=utf8mb4_uca1400_as_cs;"
	got, warns := preprocessWith(t, sql)
	s := squash(got)
	assert.Contains(t, s, "COLLATE utf8mb4_0900_ai_ci */")
	assert.Contains(t, s, "collation_connection = utf8mb4_0900_ai_ci")
	assert.Contains(t, s, "collation_connection = utf8mb3_bin")
	assert.Contains(t, s, "COLLATE utf8mb4_0900_ai_ci, b")
	assert.Contains(t, s, "CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci)", "a mapped charset gets the target collation")
	assert.Contains(t, s, "DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin", "case-sensitive stays case-sensitive")
	joined := strings.Join(warns, "\n")
	assert.Contains(t, joined, "collation utf8mb4_uca1400_as_ci mapped to utf8mb4_0900_ai_ci: no exact TiDB equivalent, closest one used; accent-sensitive values become equal")
	assert.Contains(t, joined, "collation latin1_swedish_ci mapped to utf8mb4_0900_ai_ci: its language-specific equality and order rules are lost")
	for _, w := range warns {
		if strings.Contains(w, "uca1400") {
			assert.NotContains(t, w, "language-specific", "uca1400 is generic")
		}
	}

	_, err := NewLoader().ParseTiDB(got)
	require.NoError(t, err)
}

func TestPreprocessKeepsSetNamesCharset(t *testing.T) {
	// SET NAMES describes the bytes of the dump itself; remapping it would
	// reinterpret every string literal.
	got, _ := preprocessWith(t, "/*!40101 SET NAMES latin1 */;")
	assert.Contains(t, got, "SET NAMES latin1")
}

func TestPreprocessSetvalFoldsIntoStart(t *testing.T) {
	sql := "CREATE SEQUENCE `s1` start with 100 minvalue 1 maxvalue 9223372036854775806 increment by 5 cache 1000 nocycle ENGINE=InnoDB;\n" +
		"DO SETVAL(`s1`, 5100, 0);\n" +
		"CREATE SEQUENCE `s2` start with 100 minvalue -9223372036854775807 maxvalue 100 increment by -5 cache 1000 nocycle ENGINE=InnoDB;\n" +
		"DO SETVAL(`s2`, -4900, 0);\n" +
		"CREATE SEQUENCE s3 start with 1 increment by 1;\n" +
		"SELECT SETVAL(s3, 41, 1);\n" +
		"DO SETVAL(`other`, 7, 0);"
	got, warns := preprocessWith(t, sql)
	s := squash(got)
	assert.Contains(t, s, "`s1` start with 5100 ")
	assert.Contains(t, s, "`s2` start with -4900 ")
	assert.Contains(t, s, "s3 start with 42 ")
	assert.Contains(t, s, "DO SETVAL(`other`, 7)")
	assert.Equal(t, 1, strings.Count(s, "SETVAL"), "folded SETVALs are dropped")
	assert.Len(t, warns, 1)

	_, err := NewLoader().ParseTiDB(got)
	require.NoError(t, err)
}

func TestPreprocessAlterTable(t *testing.T) {
	got, _ := preprocessWith(t, "ALTER TABLE t ADD COLUMN u uuid NOT NULL, MODIFY ip inet6, CHANGE a b inet4, ADD COLUMN (x uuid, y point), ADD INDEX uuid (u), PAGE_COMPRESSED=1")
	s := squash(got)
	assert.Contains(t, s, "ADD COLUMN u char(36) NOT NULL DEFAULT ''")
	assert.Contains(t, s, "MODIFY ip varchar(39)")
	assert.Contains(t, s, "CHANGE a b varchar(15)")
	assert.Contains(t, s, "(x char(36), y longblob)")
	assert.Contains(t, s, "ADD INDEX uuid (u)")
	assert.NotContains(t, s, "PAGE_COMPRESSED")
	_, err := NewLoader().ParseTiDB(got)
	require.NoError(t, err)
}
