package parser

import (
	"strings"
	"testing"
)

// TestLoaderPreprocessUUID ensures that UUID column types are converted to char(36)
// during preprocessing while UUID() functions remain untouched.
func TestLoaderPreprocessUUID(t *testing.T) {
	loader := NewLoader()
	sql := "CREATE TABLE t (id uuid NOT NULL DEFAULT uuid());"
	stmts, err := loader.LoadFromString(sql)
	if err != nil {
		t.Fatalf("failed to parse sql: %v", err)
	}

	writer := NewWriter()
	out, err := writer.WriteStatements(stmts)
	if err != nil {
		t.Fatalf("failed to write sql: %v", err)
	}

	lower := strings.ToLower(out)
	if strings.Contains(lower, "uuid not null") {
		t.Errorf("uuid type not transformed: %s", out)
	}
	if !strings.Contains(lower, "char(36) not null") {
		t.Errorf("char(36) not found: %s", out)
	}
	if !strings.Contains(lower, "default uuid()") && !strings.Contains(lower, "default (uuid())") {
		t.Errorf("uuid() function should remain, got: %s", out)
	}
}

// TestLoaderPreprocessUUIDColumnName ensures that a column named uuid retains
// its name while its type is converted to char(36).
func TestLoaderPreprocessUUIDColumnName(t *testing.T) {
	loader := NewLoader()
	sql := "CREATE TABLE t (uuid uuid NOT NULL);"
	stmts, err := loader.LoadFromString(sql)
	if err != nil {
		t.Fatalf("failed to parse sql: %v", err)
	}

	writer := NewWriter()
	out, err := writer.WriteStatements(stmts)
	if err != nil {
		t.Fatalf("failed to write sql: %v", err)
	}

	lower := strings.ToLower(out)
	cleaned := strings.ReplaceAll(lower, "`", "")
	if strings.Count(cleaned, "uuid") != 1 {
		t.Errorf("uuid column name should appear once, got: %s", out)
	}
	if !strings.Contains(cleaned, "uuid char(36)") {
		t.Errorf("uuid type not converted or column name changed: %s", out)
	}
}

// TestLoaderPreprocessUUIDIndexName ensures that indexes using UUID
// retain their names and references after preprocessing.
func TestLoaderPreprocessUUIDIndexName(t *testing.T) {
	loader := NewLoader()
	sql := "CREATE TABLE t (id int, uuid uuid NOT NULL, UNIQUE KEY `uuid` (`uuid`));"
	stmts, err := loader.LoadFromString(sql)
	if err != nil {
		t.Fatalf("failed to parse sql: %v", err)
	}

	writer := NewWriter()
	out, err := writer.WriteStatements(stmts)
	if err != nil {
		t.Fatalf("failed to write sql: %v", err)
	}

	lower := strings.ToLower(out)
	cleaned := strings.ReplaceAll(lower, "`", "")

	if !strings.Contains(cleaned, "uuid_key") {
		t.Errorf("index name not renamed: %s", out)
	}
	if strings.Count(cleaned, "uuid") != 3 {
		t.Errorf("expected uuid to appear three times, got: %s", out)
	}
	if !strings.Contains(cleaned, "uuid char(36)") {
		t.Errorf("uuid type not converted: %s", out)
	}
}

// TestLoaderUUIDNotNullDefault ensures that UUID columns marked as NOT NULL
// receive an explicit empty string default to maintain compatibility.
func TestLoaderUUIDNotNullDefault(t *testing.T) {
	loader := NewLoader()
	sql := "CREATE TABLE t (id int, uuid uuid NOT NULL);"
	stmts, err := loader.LoadFromString(sql)
	if err != nil {
		t.Fatalf("failed to parse sql: %v", err)
	}

	writer := NewWriter()
	out, err := writer.WriteStatements(stmts)
	if err != nil {
		t.Fatalf("failed to write sql: %v", err)
	}

	lower := strings.ToLower(out)
	cleaned := strings.ReplaceAll(lower, "`", "")
	if !strings.Contains(cleaned, "uuid char(36) not null default ''") {
		t.Errorf("missing default empty string for uuid column: %s", out)
	}
}

// TestLoaderPreprocessEncryptedOptions ensures table encryption options are removed
// during preprocessing so the SQL can be parsed by TiDB.
func TestLoaderPreprocessEncryptedOptions(t *testing.T) {
	loader := NewLoader()
	sql := "CREATE TABLE t (id int) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 `encrypted`=yes `encryption_key_id`=1;"
	stmts, err := loader.LoadFromString(sql)
	if err != nil {
		t.Fatalf("failed to parse sql: %v", err)
	}

	writer := NewWriter()
	out, err := writer.WriteStatements(stmts)
	if err != nil {
		t.Fatalf("failed to write sql: %v", err)
	}

	lower := strings.ToLower(out)
	if strings.Contains(lower, "`encrypted`") || strings.Contains(lower, "`encryption_key_id`") {
		t.Errorf("encrypted options not removed: %s", out)
	}
}
