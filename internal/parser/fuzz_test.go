package parser

import (
	"testing"
)

// FuzzPreprocessSQL exercises the textual preprocessing (UUID rewriting,
// encryption-option stripping, charset mappings) with arbitrary input.
// It must never panic, whatever bytes arrive.
func FuzzPreprocessSQL(f *testing.F) {
	seeds := []string{
		"",
		"uuid",
		"`uuid`",
		"CREATE TABLE t (id uuid NOT NULL, UNIQUE KEY `uuid` (id));",
		"CREATE TABLE t (c char(36) NOT NULL);",
		"CREATE TABLE t (c char(36) NOT NULL default 'x', d uuid);",
		"CREATE TABLE t (n int) `encrypted`=yes `encryption_key_id`=1;",
		"select uuid();",
		"CREATE TABLE t (v varchar(10) CHARACTER SET latin1 COLLATE latin1_swedish_ci);",
		"uuid uuid uuid char(36) not null",
		"char(36)char(36)",
		// mariadb-dump shapes: DELIMITER blocks, executable and MariaDB-only
		// comments, sequences, MariaDB-only attributes and options.
		"DELIMITER ;;\n/*!50003 CREATE*/ /*!50017 DEFINER=`root`@`%`*/ /*!50003 TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET NEW.a=1; END */;;\nDELIMITER ;\n",
		"/*M!999999\\- enable the sandbox mode */ SET collation_connection = utf8mb4_uca1400_ai_ci;",
		"CREATE SEQUENCE s start with 1 increment by -5 maxvalue 1; DO SETVAL(`s`, -9, 0, 1);",
		"CREATE TABLE t (a INT INVISIBLE, b BLOB COMPRESSED=zlib, g POINT, SPATIAL KEY (g), PERIOD FOR p(a, a)) `PAGE_COMPRESSED`='1', WITH SYSTEM VERSIONING",
		"ALTER TABLE t ADD COLUMN (a uuid, b inet6), PAGE_COMPRESSED=1,",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	loader := NewLoader().WithCharsetMappings(
		map[string]string{"latin1": "utf8mb4"},
		map[string]string{"latin1_swedish_ci": "utf8mb4_0900_ai_ci"},
	)
	f.Fuzz(func(_ *testing.T, sql string) {
		_ = loader.preprocessSQL(sql)
	})
}

// FuzzLoadFromString feeds arbitrary input through preprocessing plus the
// TiDB parser. Parse errors are expected; panics are not.
func FuzzLoadFromString(f *testing.F) {
	seeds := []string{
		"CREATE TABLE t (id INT PRIMARY KEY);",
		"CREATE TABLE t (u uuid NOT NULL, KEY k (u));",
		"CREATE DATABASE d CHARACTER SET latin1;",
		"not sql at all",
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	loader := NewLoader()
	f.Fuzz(func(_ *testing.T, sql string) {
		_, _ = loader.LoadFromString(sql)
	})
}
