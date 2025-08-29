package parser

import (
	"strings"
	"testing"

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
