// Package test contains end-to-end golden tests: every fixture in
// test/fixtures/<name>.sql is run through the full load → transform → write
// pipeline and compared against test/testdata/<name>.expected.sql.
//
// Regenerate the expected files after intentional behavior changes with:
//
//	go test ./test -run TestGolden -update
package test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/parser"
	"github.com/developer-Bushido/mariadb2tidb/internal/transformer"
)

var update = flag.Bool("update", false, "rewrite golden files with current pipeline output")

func TestGolden(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("fixtures", "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, fixtures, "no fixtures found")

	cfg := config.DefaultConfig()

	for _, fixture := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fixture), ".sql")
		t.Run(name, func(t *testing.T) {
			loader := parser.NewLoaderWithConfig(cfg)
			stmts, err := loader.LoadFromFile(fixture)
			require.NoError(t, err, "load fixture")

			engine := transformer.NewEngine(cfg)
			transformed, err := engine.Transform(stmts)
			require.NoError(t, err, "transform")

			writer := parser.NewWriter()
			got, err := writer.WriteStatements(transformed)
			require.NoError(t, err, "write")
			got += "\n"

			// The output must itself be valid SQL for the TiDB parser.
			_, err = parser.NewLoader().LoadFromString(got)
			require.NoError(t, err, "transformed output must re-parse with the TiDB parser")

			goldenPath := filepath.Join("testdata", name+".expected.sql")
			if *update {
				require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
				return
			}

			want, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "read golden file (run with -update to create)")
			assert.Equal(t, string(want), got)
		})
	}
}
