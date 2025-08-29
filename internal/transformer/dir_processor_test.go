package transformer

import (
	"context"
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
