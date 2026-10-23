package transformer

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
)

func TestProcessDirectory(t *testing.T) {
	require.NoError(t, utils.InitLogger(true))

	inputDir := t.TempDir()
	outputDir := t.TempDir()

	sql1 := "CREATE TABLE t1 (name VARCHAR(10) COLLATE latin1_swedish_ci) DEFAULT CHARSET=latin1;"
	sql2 := "CREATE TABLE t2 (id INT);"

	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "a.sql"), []byte(sql1), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "b.sql"), []byte(sql2), 0o644))

	p := NewDirProcessor(config.DefaultConfig())
	err := p.ProcessDirectory(context.Background(), inputDir, outputDir, 2)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(outputDir, "a.sql"))
	require.NoError(t, err)
	str := string(data)
	assert.NotContains(t, str, "latin1_swedish_ci")
	assert.Contains(t, str, "utf8mb4_0900_ai_ci")

	data2, err := os.ReadFile(filepath.Join(outputDir, "b.sql"))
	require.NoError(t, err)
	assert.Contains(t, strings.ToUpper(string(data2)), "T2")
}

func TestProcessDirectoryNilConfig(t *testing.T) {
	require.NoError(t, utils.InitLogger(true))

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(inputDir, "a.sql"), []byte("CREATE TABLE t (id INT);"), 0o644))

	p := NewDirProcessor(nil)
	require.NoError(t, p.ProcessDirectory(context.Background(), inputDir, outputDir, 1))

	data, err := os.ReadFile(filepath.Join(outputDir, "a.sql"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "CREATE TABLE `t`")
}

func TestProcessDirectoryDumplingLayout(t *testing.T) {
	require.NoError(t, utils.InitLogger(true))

	in, out := t.TempDir(), t.TempDir()
	files := map[string]string{
		"metadata":                        "Started dump at: 2026-10-22\n",
		"shop-schema-create.sql":          "CREATE DATABASE `shop` /*!40100 DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_uca1400_ai_ci */;\n",
		"shop.items-schema.sql":           "CREATE TABLE `items` (`id` int NOT NULL, `u` uuid NOT NULL, PRIMARY KEY (`id`)) ENGINE=Aria;\n",
		"shop.items.000000000.sql":        "INSERT INTO `items` VALUES (1,'a uuid in data, ENGINE=Aria');\n",
		"shop.items.000000001.csv":        "2,\"x\"\n",
		"shop.items.0000000010000.sql.gz": "\x1f\x8b binary",
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(in, name), []byte(body), 0o644))
	}

	p := NewDirProcessor(config.DefaultConfig())
	require.NoError(t, p.ProcessDirectory(context.Background(), in, out, 2))

	for _, name := range []string{"metadata", "shop.items.000000000.sql", "shop.items.000000001.csv", "shop.items.0000000010000.sql.gz"} {
		got, err := os.ReadFile(filepath.Join(out, name))
		require.NoError(t, err, name)
		assert.Equal(t, files[name], string(got), "%s must be copied byte for byte", name)
	}
	schema, err := os.ReadFile(filepath.Join(out, "shop.items-schema.sql"))
	require.NoError(t, err)
	assert.Contains(t, string(schema), "CHAR(36)")
	assert.Contains(t, string(schema), "ENGINE = InnoDB")
	db, err := os.ReadFile(filepath.Join(out, "shop-schema-create.sql"))
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(string(db)), "utf8mb4_0900_ai_ci")

	assert.Contains(t, p.Warnings(), "shop.items-schema.sql: table items: ENGINE=Aria replaced with InnoDB (not supported by TiDB)")
	assert.Contains(t, strings.Join(p.Warnings(), "\n"), "shop-schema-create.sql: collation utf8mb4_uca1400_ai_ci mapped to utf8mb4_0900_ai_ci")
}

func TestProcessDirectoryRefusesUnsafeLayouts(t *testing.T) {
	require.NoError(t, utils.InitLogger(true))
	p := NewDirProcessor(config.DefaultConfig())
	ctx := context.Background()

	in := t.TempDir()
	data := filepath.Join(in, "db.t.000000000.sql")
	require.NoError(t, os.WriteFile(data, []byte("INSERT INTO t VALUES (1);"), 0o644))
	assert.Error(t, p.ProcessDirectory(ctx, in, in, 1), "output = input")
	assert.Error(t, p.ProcessDirectory(ctx, in, filepath.Join(in, "out"), 1), "output inside input")
	assert.Error(t, p.ProcessDirectory(ctx, filepath.Join(in), filepath.Dir(in), 1), "input inside output")
	got, err := os.ReadFile(data)
	require.NoError(t, err)
	assert.Equal(t, "INSERT INTO t VALUES (1);", string(got), "input untouched")

	// A symlink inside the tree fails instead of being skipped.
	other := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(other, "real.sql"), []byte("CREATE TABLE r (id INT);"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(other, "real.sql"), filepath.Join(in, "db.r.000000001.sql")))
	assert.ErrorContains(t, p.ProcessDirectory(ctx, in, t.TempDir(), 1), "symbolic link")

	// A symlinked input root is followed.
	root := filepath.Join(t.TempDir(), "rootlink")
	require.NoError(t, os.Symlink(other, root))
	out := t.TempDir()
	require.NoError(t, p.ProcessDirectory(ctx, root, out, 1))
	_, err = os.Stat(filepath.Join(out, "real.sql"))
	assert.NoError(t, err)
}

func TestProcessDirectoryCompressedSchema(t *testing.T) {
	require.NoError(t, utils.InitLogger(true))
	in, out := t.TempDir(), t.TempDir()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("CREATE TABLE u (id INT) ENGINE=Aria;"))
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(filepath.Join(in, "db.u-schema.sql.gz"), buf.Bytes(), 0o644))

	require.NoError(t, NewDirProcessor(nil).ProcessDirectory(context.Background(), in, out, 1))
	f, err := os.Open(filepath.Join(out, "db.u-schema.sql.gz"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	got, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Contains(t, string(got), "ENGINE = InnoDB")

	require.NoError(t, os.WriteFile(filepath.Join(in, "db.v-schema.sql.zst"), []byte("x"), 0o644))
	assert.ErrorContains(t, NewDirProcessor(nil).ProcessDirectory(context.Background(), in, t.TempDir(), 1), "decompress")
}

func TestDumplingDataFileNames(t *testing.T) {
	for name, data := range map[string]bool{
		"db.t.000000000.sql":        true,
		"db.t.0000000010000.csv.gz": true,
		"db.t.000000001.sql.zst":    true,
		"shop.20261022.sql":         false,
		"backup.1.sql":              false,
		"db.t-schema.sql":           false,
		"db.t.12.sql":               false,
		"db-schema-create.sql":      false,
	} {
		assert.Equal(t, data, dumplingDataFile.MatchString(name), name)
	}
}

func TestProcessDirectoryRefusesSameDirUnderAnotherName(t *testing.T) {
	require.NoError(t, utils.InitLogger(true))
	parent := t.TempDir()
	in := filepath.Join(parent, "Dump")
	require.NoError(t, os.Mkdir(in, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(in, "db.t.000000000.sql"), []byte("INSERT INTO t VALUES (1);"), 0o644))
	alias := filepath.Join(parent, "dump")
	if _, err := os.Stat(alias); err != nil {
		t.Skip("case-sensitive file system")
	}
	assert.Error(t, NewDirProcessor(nil).ProcessDirectory(context.Background(), in, alias, 1))
	got, err := os.ReadFile(filepath.Join(in, "db.t.000000000.sql"))
	require.NoError(t, err)
	assert.Equal(t, "INSERT INTO t VALUES (1);", string(got))
}

func TestProcessDirectoryRefusesLinkedOutputSubdir(t *testing.T) {
	require.NoError(t, utils.InitLogger(true))
	in, out := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(in, "sub"), 0o755))
	schema := filepath.Join(in, "sub", "db.t-schema.sql")
	require.NoError(t, os.WriteFile(schema, []byte("CREATE TABLE t (id INT) ENGINE=Aria;"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(in, "sub"), filepath.Join(out, "sub")))

	assert.ErrorContains(t, NewDirProcessor(nil).ProcessDirectory(context.Background(), in, out, 1), "symbolic link")
	got, _ := os.ReadFile(schema)
	assert.Equal(t, "CREATE TABLE t (id INT) ENGINE=Aria;", string(got), "input untouched")
}

func TestProcessDirectoryDoesNotWriteThroughLinks(t *testing.T) {
	require.NoError(t, utils.InitLogger(true))
	in, out := t.TempDir(), t.TempDir()
	data := filepath.Join(in, "db.t.000000000.sql")
	schema := filepath.Join(in, "db.t-schema.sql")
	require.NoError(t, os.WriteFile(data, []byte("INSERT INTO t VALUES (1);"), 0o644))
	require.NoError(t, os.WriteFile(schema, []byte("CREATE TABLE t (id INT) ENGINE=Aria;"), 0o644))
	// Leftovers in the output that point back at the input.
	require.NoError(t, os.Link(data, filepath.Join(out, "db.t.000000000.sql")))
	require.NoError(t, os.Symlink(schema, filepath.Join(out, "db.t-schema.sql")))

	require.NoError(t, NewDirProcessor(nil).ProcessDirectory(context.Background(), in, out, 1))
	got, _ := os.ReadFile(data)
	assert.Equal(t, "INSERT INTO t VALUES (1);", string(got), "input data untouched")
	got, _ = os.ReadFile(schema)
	assert.Equal(t, "CREATE TABLE t (id INT) ENGINE=Aria;", string(got), "input schema untouched")
	got, _ = os.ReadFile(filepath.Join(out, "db.t-schema.sql"))
	assert.Contains(t, string(got), "ENGINE = InnoDB")
}
