package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigKeepsDefaultsForOmittedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	require.NoError(t, os.WriteFile(path, []byte("auto_random:\n  tables: [orders]\nshard_row_id_bits: 4\nstrict_mode: true\n"), 0o644))
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, AutoRandomConfig{Tables: []string{"orders"}, ShardBits: 5, RangeBits: 54}, cfg.AutoRandom, "omitted nested fields keep their defaults")
	assert.Equal(t, uint64(4), cfg.ShardRowIDBits)
	assert.Equal(t, "keep", cfg.ZeroDateDefaults)
	assert.Equal(t, DefaultConfig().CollationMappings, cfg.CollationMappings)
}

func TestLoadConfigErrors(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	assert.Error(t, err)
	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("auto_random: [not, a, map]\n"), 0o644))
	_, err = LoadConfig(path)
	assert.Error(t, err)
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig(), cfg)
}

func TestIsRuleEnabled(t *testing.T) {
	cfg := DefaultConfig()
	assert.True(t, cfg.IsRuleEnabled("Collation"))
	assert.False(t, cfg.IsRuleEnabled("JsonGenerated"))
	cfg.EnabledRules = []string{"Engine"}
	assert.True(t, cfg.IsRuleEnabled("Engine"))
	assert.False(t, cfg.IsRuleEnabled("Collation"))
}

// TestDefaultYAMLMatchesDefaultConfig keeps the documented config file and
// the built-in defaults from drifting apart.
func TestDefaultYAMLMatchesDefaultConfig(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "config", "default.yaml"))
	require.NoError(t, err)
	cfg.InputDir, cfg.OutputDir = "", "" // example paths only
	assert.Equal(t, DefaultConfig(), cfg)
}

func TestLoadConfigRejectsTyposAndBadValues(t *testing.T) {
	cases := map[string]string{
		"unknown key":       "zero_date_default: drop\n",
		"bad enum":          "zero_date_defaults: dorp\n",
		"shard bits":        "auto_random:\n  shard_bits: 16\n",
		"range bits":        "auto_random:\n  range_bits: 70\n",
		"too few incr bits": "auto_random:\n  shard_bits: 15\n  range_bits: 40\n",
		"shard row id":      "shard_row_id_bits: 16\n",
		"pre split > shard": "shard_row_id_bits: 2\npre_split_regions: 3\n",
		"negative indent":   "formatting:\n  indent_size: -1\n",
	}
	for name, body := range cases {
		path := filepath.Join(t.TempDir(), "c.yaml")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		_, err := LoadConfig(path)
		assert.Error(t, err, name)
	}
	path := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(path, nil, 0o644))
	cfg, err := LoadConfig(path)
	require.NoError(t, err, "an empty file is the default config")
	assert.Equal(t, DefaultConfig(), cfg)
}
