//go:build integration

// Package integration runs the converter against live servers: every golden
// fixture is applied to TiDB, and a corpus of MariaDB features goes through
// a real MariaDB, mariadb-dump, the converter and TiDB, after which schema
// and data are compared.
//
// Environment (tests skip when unset):
//
//	M2T_TIDB_DSN     e.g. root@tcp(127.0.0.1:14000)/
//	M2T_MARIADB_DSN  e.g. root@tcp(127.0.0.1:13306)/
//	M2T_MARIADB_DUMP command prefix running mariadb-dump against that
//	                 server, e.g. "docker exec m2t-mariadb mariadb-dump -uroot"
//
// Run with: make integration (starts the containers) or
// go test -tags integration ./test/integration
package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/parser"
	"github.com/developer-Bushido/mariadb2tidb/internal/transformer"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
)

const corpusDB = "m2t_corpus"

// open connects to dsn (with or without a trailing database), waiting up to
// a minute for the server to accept connections. charset=utf8mb4 matters:
// a client asking for a collation the server lacks silently falls back to
// latin1 and double-encodes test data.
func open(t *testing.T, env string) *sql.DB {
	t.Helper()
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skipf("%s not set", env)
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := sql.Open("mysql", dsn+sep+"multiStatements=true&charset=utf8mb4&collation=utf8mb4_general_ci")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	deadline := time.Now().Add(time.Minute)
	for {
		if err = db.Ping(); err == nil {
			return db
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: server not reachable: %v", env, err)
		}
		time.Sleep(2 * time.Second)
	}
}

// execScript runs a script on one connection so SET and USE carry over.
func execScript(t *testing.T, db *sql.DB, script string) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	for _, stmt := range parser.SplitScript(script) {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("statement failed: %v\n%s", err, stmt)
		}
	}
}

func transform(t *testing.T, cfg *config.Config, sqlText string) (string, []string) {
	t.Helper()
	require.NoError(t, utils.InitLogger(false))
	rep := compat.NewReporter()
	out, err := transformer.TransformString(cfg, sqlText, rep)
	require.NoError(t, err)
	return out, rep.Warnings()
}

// TestFixturesApplyOnTiDB applies every golden fixture's output to TiDB in
// its default (strict) sql_mode.
func TestFixturesApplyOnTiDB(t *testing.T) {
	tidb := open(t, "M2T_TIDB_DSN")
	fixtures, err := filepath.Glob(filepath.Join("..", "fixtures", "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, fixtures)
	for _, f := range fixtures {
		name := strings.TrimSuffix(filepath.Base(f), ".sql")
		t.Run(name, func(t *testing.T) {
			in, err := os.ReadFile(f)
			require.NoError(t, err)
			out, _ := transform(t, config.DefaultConfig(), string(in))
			db := "m2t_fix_" + name
			execScript(t, tidb, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s`; USE `%s`;\n%s", db, db, db, out))
		})
	}
}

// TestMariaDBDumpToTiDB is the end-to-end check: real MariaDB, real
// mariadb-dump, converted schema, unconverted data, compared on TiDB.
func TestMariaDBDumpToTiDB(t *testing.T) {
	maria := open(t, "M2T_MARIADB_DSN")
	tidb := open(t, "M2T_TIDB_DSN")
	dump := strings.Fields(os.Getenv("M2T_MARIADB_DUMP"))
	if len(dump) == 0 {
		t.Skip("M2T_MARIADB_DUMP not set")
	}

	corpus, err := os.ReadFile(filepath.Join("testdata", "mariadb_corpus.sql"))
	require.NoError(t, err)
	execScript(t, maria, fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s; USE %s;\n%s", corpusDB, corpusDB, corpusDB, corpus))
	// Use some sequence values so the dump has to carry a non-initial state,
	// and exhaust one.
	lastUp, lastDown := nextvals(t, maria, 3)
	for range 3 {
		_, err := maria.Exec("SELECT NEXTVAL(" + corpusDB + ".seq_exhausted)")
		require.NoError(t, err)
	}

	runDump := func(args ...string) string {
		cmd := exec.Command(dump[0], append(dump[1:], args...)...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		require.NoError(t, err, "mariadb-dump: %s", stderr.String())
		return string(out)
	}
	// One full dump, schema and data, as people migrate with mariadb-dump:
	// the data goes through the converter too and must come out unchanged.
	full := runDump("--routines", "--triggers", "--events", "--databases", corpusDB)

	out, warns := transform(t, config.DefaultConfig(), full)
	execScript(t, tidb, "DROP DATABASE IF EXISTS "+corpusDB+";\n"+out)

	t.Run("warnings", func(t *testing.T) {
		joined := strings.Join(warns, "\n")
		for _, want := range []string{
			"dropped TRIGGER trg_plain", "dropped PROCEDURE p1", "dropped FUNCTION f1", "dropped EVENT ev1",
			"ENGINE=Aria replaced", "WITH SYSTEM VERSIONING", "is no longer INVISIBLE", "POINT became LONGBLOB",
			"JSON_VALUE rewritten", "zero-date DEFAULT",
			"values for generated columns b, c, jv replaced with DEFAULT",
			"dropped PROCEDURE p_nbe", "dropped TRIGGER trg_cmt", "seq_exhausted` is exhausted",
		} {
			assert.Contains(t, joined, want)
		}
	})

	t.Run("schema", func(t *testing.T) {
		mt, tt := baseTables(t, maria), baseTables(t, tidb)
		// MariaDB reports system-versioned tables with their own table_type.
		assert.Equal(t, append(mt, "versioned"), tt)
		mc, tc := columns(t, maria), columns(t, tidb)
		require.Equal(t, keys(mc), keys(tc), "same columns in the same order")
		for k, m := range mc {
			assert.Equal(t, m.nullable, tc[k].nullable, "%s nullability", k)
		}
		assert.Equal(t, indexes(t, maria), indexes(t, tidb))
	})

	t.Run("data", func(t *testing.T) {
		for _, table := range baseTables(t, maria) {
			assert.Equal(t, rows(t, maria, table), rows(t, tidb, table), "rows of %s, byte for byte", table)
		}
		assert.Equal(t, rows(t, maria, "v_plain"), rows(t, tidb, "v_plain"), "view")
	})

	t.Run("sequences", func(t *testing.T) {
		up, down := nextvals(t, tidb, 1)
		assert.Greater(t, up, lastUp, "ascending sequence must not repeat issued values")
		assert.Less(t, down, lastDown, "descending sequence must not repeat issued values")
		_, err := tidb.Exec("SELECT NEXTVAL(" + corpusDB + ".seq_exhausted)")
		assert.Error(t, err, "an exhausted sequence stays exhausted")
	})

	t.Run("collations", func(t *testing.T) {
		var cs, ci int
		require.NoError(t, tidb.QueryRow("SELECT COUNT(*) FROM "+corpusDB+".uca WHERE cs = 'abc'").Scan(&cs))
		require.NoError(t, tidb.QueryRow("SELECT COUNT(*) FROM "+corpusDB+".uca WHERE ci = 'ETE'").Scan(&ci))
		assert.Equal(t, 0, cs, "as_cs stays case-sensitive")
		assert.Equal(t, 1, ci, "ai_ci stays case- and accent-insensitive")
	})
}

// TestStrictModeWithZeroDateDrop applies the dump without its permissive
// SQL_MODE header: zero-date defaults must then be dropped to load.
func TestStrictModeWithZeroDateDrop(t *testing.T) {
	tidb := open(t, "M2T_TIDB_DSN")
	in := "CREATE TABLE z (a DATETIME NOT NULL DEFAULT '0000-00-00 00:00:00', b DATE DEFAULT '0000-00-00');"
	cfg := config.DefaultConfig()
	cfg.ZeroDateDefaults = "drop"
	out, _ := transform(t, cfg, in)
	execScript(t, tidb, "DROP DATABASE IF EXISTS m2t_strict; CREATE DATABASE m2t_strict; USE m2t_strict;\n"+out)

	kept, _ := transform(t, config.DefaultConfig(), in)
	_, err := tidb.Exec("USE m2t_strict; DROP TABLE z; " + kept)
	assert.ErrorContains(t, err, "1067", "kept zero dates fail under strict sql_mode, as the warning says")
}

// TestAutoRandomOnTiDB checks the generated AUTO_RANDOM keys stay below 2^53.
func TestAutoRandomOnTiDB(t *testing.T) {
	tidb := open(t, "M2T_TIDB_DSN")
	cfg := config.DefaultConfig()
	cfg.AutoRandom.Tables = []string{"*"}
	cfg.ShardRowIDBits = 4
	out, _ := transform(t, cfg, "CREATE TABLE ids (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, v INT); CREATE TABLE logs (msg TEXT);")
	execScript(t, tidb, "DROP DATABASE IF EXISTS m2t_ar; CREATE DATABASE m2t_ar; USE m2t_ar;\n"+out+
		"INSERT INTO ids (v) VALUES (1),(2),(3),(4),(5),(6),(7),(8);")
	var over int
	require.NoError(t, tidb.QueryRow("SELECT COUNT(*) FROM m2t_ar.ids WHERE id >= POWER(2, 53)").Scan(&over))
	assert.Zero(t, over)
	var sharding string
	require.NoError(t, tidb.QueryRow("SELECT tidb_row_id_sharding_info FROM information_schema.tables WHERE table_schema='m2t_ar' AND table_name='logs'").Scan(&sharding))
	assert.Equal(t, "SHARD_BITS=4", sharding)
}

func nextvals(t *testing.T, db *sql.DB, n int) (up, down int64) {
	t.Helper()
	for range n {
		require.NoError(t, db.QueryRow("SELECT NEXTVAL("+corpusDB+".seq_up), NEXTVAL("+corpusDB+".seq_down)").Scan(&up, &down))
	}
	return up, down
}

func baseTables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	return strings1(t, db, "SELECT table_name FROM information_schema.tables WHERE table_schema=? AND table_type='BASE TABLE' ORDER BY 1", corpusDB)
}

func strings1(t *testing.T, db *sql.DB, q string, args ...any) []string {
	t.Helper()
	r, err := db.Query(q, args...)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	var out []string
	for r.Next() {
		var s string
		require.NoError(t, r.Scan(&s))
		out = append(out, strings.ToLower(s))
	}
	require.NoError(t, r.Err())
	return out
}

type column struct{ nullable string }

func columns(t *testing.T, db *sql.DB) map[string]column {
	t.Helper()
	r, err := db.Query(`SELECT c.table_name, c.ordinal_position, c.column_name, c.is_nullable
		FROM information_schema.columns c JOIN information_schema.tables t
		  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = ? AND t.table_type IN ('BASE TABLE', 'SYSTEM VERSIONED')`, corpusDB)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	out := map[string]column{}
	for r.Next() {
		var table, name, nullable string
		var pos int
		require.NoError(t, r.Scan(&table, &pos, &name, &nullable))
		out[fmt.Sprintf("%s.%02d.%s", strings.ToLower(table), pos, strings.ToLower(name))] = column{nullable}
	}
	require.NoError(t, r.Err())
	return out
}

func keys(m map[string]column) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// indexes lists name, columns and uniqueness of every index, sorted here:
// the servers would sort the strings under different collations.
func indexes(t *testing.T, db *sql.DB) []string {
	t.Helper()
	out := strings1(t, db, `SELECT CONCAT(table_name, '.', index_name, '(', GROUP_CONCAT(column_name ORDER BY seq_in_index), ') unique=', 1 - non_unique)
		FROM information_schema.statistics WHERE table_schema = ? GROUP BY table_name, index_name, non_unique`, corpusDB)
	sort.Strings(out)
	return out
}

// textTypes are compared as characters: their bytes legitimately change
// with the charset (latin1 'Å' is C5, utf8mb4 is C385), and MariaDB stores
// UUID/INET values in binary but dumps and converts them as text.
var fractionalTypes = map[string]bool{"decimal": true, "double": true, "float": true}

var textTypes = map[string]bool{
	"char": true, "varchar": true, "tinytext": true, "text": true, "mediumtext": true, "longtext": true,
	"enum": true, "set": true, "json": true, "uuid": true, "inet4": true, "inet6": true,
}

// rows returns every row of table as raw bytes, ordered by all columns.
func rows(t *testing.T, db *sql.DB, table string) [][]string {
	t.Helper()
	cols := strings1(t, db, "SELECT CONCAT(column_name, ' ', data_type) FROM information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position", corpusDB, table)
	quoted := make([]string, len(cols))
	order := make([]string, len(cols))
	for i, c := range cols {
		name, typ, _ := strings.Cut(c, " ")
		// HEX keeps the comparison on stored bytes (or, for text, on the
		// characters), not on how a client renders them.
		switch {
		case textTypes[typ]:
			quoted[i] = fmt.Sprintf("HEX(CONVERT(`%s` USING utf8mb4))", name)
		case fractionalTypes[typ]:
			// HEX(number) converts to an integer: 1.10 and 1.20 would
			// compare equal. Compare the text form instead.
			quoted[i] = fmt.Sprintf("HEX(CAST(`%s` AS CHAR))", name)
		default:
			quoted[i] = fmt.Sprintf("HEX(`%s`)", name)
		}
		order[i] = fmt.Sprint(i + 1)
	}
	r, err := db.Query(fmt.Sprintf("SELECT %s FROM %s.`%s` ORDER BY %s", strings.Join(quoted, ", "), corpusDB, table, strings.Join(order, ", ")))
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	var out [][]string
	for r.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		require.NoError(t, r.Scan(ptrs...))
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = v.String
			if !v.Valid {
				row[i] = "NULL"
			}
		}
		out = append(out, row)
	}
	require.NoError(t, r.Err())
	return out
}
