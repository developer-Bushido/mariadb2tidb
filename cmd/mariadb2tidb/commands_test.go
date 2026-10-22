package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCLI(t *testing.T, args ...string) error {
	t.Helper()
	rootCmd.SetArgs(append([]string{"--no-color"}, args...))
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	return rootCmd.Execute()
}

func TestCLITransformFileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.sql")
	require.NoError(t, os.WriteFile(in, []byte("CREATE TABLE `t` (`id` int, `u` uuid) ENGINE=Aria COLLATE=utf8mb4_uca1400_ai_ci;\nINSERT INTO t VALUES (1,'a uuid');\n"), 0o644))

	out := filepath.Join(dir, "out.sql")
	require.NoError(t, runCLI(t, "transform", in, "-o", out))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(got), "CHAR(36)")
	assert.Contains(t, string(got), "'a uuid'")
	assert.Contains(t, string(got), "ENGINE = InnoDB")
	require.NoError(t, runCLI(t, "validate", out))

	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "db.t-schema.sql"), []byte("CREATE TABLE t (id INT);"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "db.t.000000000.sql"), []byte("INSERT INTO t VALUES (1);"), 0o644))
	require.NoError(t, runCLI(t, "transform-dir", src, "-o", dst))
	data, err := os.ReadFile(filepath.Join(dst, "sub", "db.t.000000000.sql"))
	require.NoError(t, err)
	assert.Equal(t, "INSERT INTO t VALUES (1);", string(data))
	require.NoError(t, runCLI(t, "transform", src, "-o", filepath.Join(dir, "dst2")))
}

func TestCLIErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.sql")
	require.NoError(t, os.WriteFile(bad, []byte("CREATE TABLE ("), 0o644))
	assert.Error(t, runCLI(t, "transform", bad))
	assert.Error(t, runCLI(t, "validate", bad))
	assert.Error(t, runCLI(t, "transform", filepath.Join(dir, "missing.sql")))
	assert.Error(t, runCLI(t, "transform", dir), "directory input needs -o")
	assert.Error(t, runCLI(t, "transform", bad, "--config", filepath.Join(dir, "missing.yaml")))
	assert.NoError(t, runCLI(t, "version"))
}

func TestPrintWarnings(t *testing.T) {
	printWarnings(nil)
	printWarnings([]string{"a", "b"})
}
