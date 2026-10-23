package transformer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/developer-Bushido/mariadb2tidb/internal/parser"
)

func TestVerifyOutput(t *testing.T) {
	in := "CREATE TABLE t (a INT, b INT, c LONGTEXT CHECK (json_valid(c)), PRIMARY KEY (a), KEY kb (b), CONSTRAINT chk CHECK (a > 0)); CREATE VIEW v AS SELECT 1;"
	stmts, err := parser.NewLoader().LoadFromString(in)
	require.NoError(t, err)
	shape := ShapeOf(stmts)

	ok := map[string]string{
		"identical":       in,
		"checks dropped":  "CREATE TABLE t (a INT, b INT, c LONGTEXT, PRIMARY KEY (a), KEY kb (b)); CREATE VIEW v AS SELECT 1;",
		"types and opts":  "CREATE TABLE t (a BIGINT AUTO_RANDOM, b INT DEFAULT 1, c TEXT, PRIMARY KEY (a) CLUSTERED, KEY kb (b(10))) SHARD_ROW_ID_BITS=4; CREATE VIEW v AS SELECT 2;",
		"case of columns": "CREATE TABLE T (A INT, B INT, C LONGTEXT, PRIMARY KEY (A), KEY KB (B)); CREATE VIEW v AS SELECT 1;",
	}
	for name, out := range ok {
		assert.NoError(t, VerifyOutput(shape, out), name)
	}

	bad := map[string]string{
		"column lost":     "CREATE TABLE t (a INT, b INT, PRIMARY KEY (a), KEY kb (b)); CREATE VIEW v AS SELECT 1;",
		"columns reorder": "CREATE TABLE t (b INT, a INT, c LONGTEXT, PRIMARY KEY (a), KEY kb (b)); CREATE VIEW v AS SELECT 1;",
		"key lost":        "CREATE TABLE t (a INT, b INT, c LONGTEXT, PRIMARY KEY (a)); CREATE VIEW v AS SELECT 1;",
		"key renamed":     "CREATE TABLE t (a INT, b INT, c LONGTEXT, PRIMARY KEY (a), KEY kx (b)); CREATE VIEW v AS SELECT 1;",
		"table lost":      "CREATE TABLE u (a INT); CREATE VIEW v AS SELECT 1;",
		"statement lost":  "CREATE TABLE t (a INT, b INT, c LONGTEXT, PRIMARY KEY (a), KEY kb (b));",
		"empty table":     "CREATE TABLE t; CREATE VIEW v AS SELECT 1;",
		"view renamed":    "CREATE TABLE t (a INT, b INT, c LONGTEXT, PRIMARY KEY (a), KEY kb (b)); CREATE VIEW w AS SELECT 1;",
		"not parsable":    "CREATE TABLE t (a INT",
	}
	for name, out := range bad {
		assert.Error(t, VerifyOutput(shape, out), name)
	}
}

// TestVerifyOutputStatementLevel covers losses a per-table summary missed:
// equal table names in different databases, ALTERs, column-level keys,
// INSERT rows, partitioning, key direction and foreign key actions.
func TestVerifyOutputStatementLevel(t *testing.T) {
	in := "USE a; CREATE TABLE t (x INT PRIMARY KEY, y INT UNIQUE, KEY k (y DESC), CONSTRAINT f FOREIGN KEY (y) REFERENCES p (id) ON DELETE CASCADE) PARTITION BY HASH (x) PARTITIONS 4;" +
		"USE b; CREATE TABLE t (x INT, z INT);" +
		"ALTER TABLE t ADD COLUMN w INT, ADD KEY kw (w);" +
		"INSERT INTO t VALUES (1,2),(3,4);"
	stmts, err := parser.NewLoader().LoadFromString(in)
	require.NoError(t, err)
	shape := ShapeOf(stmts)
	require.NoError(t, VerifyOutput(shape, in))
	require.NoError(t, VerifyOutput(shape, strings.Replace(in, "x INT PRIMARY KEY", "x BIGINT PRIMARY KEY CLUSTERED", 1)), "types and clustering may change")

	bad := map[string][2]string{
		"column lost in second db": {"(x INT, z INT)", "(x INT)"},
		"column-level PK lost":     {"x INT PRIMARY KEY", "x INT"},
		"column-level UNIQUE lost": {"y INT UNIQUE", "y INT"},
		"key direction lost":       {"(y DESC)", "(y)"},
		"FK action lost":           {"ON DELETE CASCADE", ""},
		"partitioning lost":        {" PARTITION BY HASH (x) PARTITIONS 4", ""},
		"ALTER column lost":        {"ADD COLUMN w INT, ", ""},
		"ALTER key lost":           {", ADD KEY kw (w)", ""},
		"INSERT row lost":          {",(3,4)", ""},
		"INSERT retargeted":        {"INSERT INTO t", "INSERT INTO u"},
		"USE changed":              {"USE b", "USE c"},
	}
	for name, r := range bad {
		out := strings.Replace(in, r[0], r[1], 1)
		require.NotEqual(t, in, out, name)
		assert.Error(t, VerifyOutput(shape, out), name)
	}
}

func TestVerifyOutputIndexesAndRenames(t *testing.T) {
	in := "CREATE INDEX i ON t (a, b DESC); DROP INDEX j ON t; ALTER TABLE t RENAME COLUMN a TO b, RENAME INDEX x TO y, RENAME TO u, DROP INDEX k;" +
		"RENAME TABLE p TO q; CREATE VIEW v (c1, c2) AS SELECT 1, 2;"
	stmts, err := parser.NewLoader().LoadFromString(in)
	require.NoError(t, err)
	shape := ShapeOf(stmts)
	require.NoError(t, VerifyOutput(shape, in))
	for name, r := range map[string][2]string{
		"index column lost":    {"(a, b DESC)", "(a)"},
		"index direction":      {"b DESC", "b"},
		"drop index renamed":   {"DROP INDEX j", "DROP INDEX z"},
		"rename column target": {"TO b,", "TO c,"},
		"rename index target":  {"TO y", "TO z"},
		"rename table target":  {"RENAME TO u", "RENAME TO w"},
		"alter drop index":     {"DROP INDEX k", "DROP INDEX kk"},
		"rename table stmt":    {"TO q", "TO r"},
		"view columns":         {"(c1, c2)", "(c1, c3)"},
	} {
		out := strings.Replace(in, r[0], r[1], 1)
		require.NotEqual(t, in, out, name)
		assert.Error(t, VerifyOutput(shape, out), name)
	}
}
