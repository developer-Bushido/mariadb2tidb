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
