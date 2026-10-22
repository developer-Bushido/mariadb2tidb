package compat

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
)

func TestResolveCollation(t *testing.T) {
	r := NewCollationResolver(config.DefaultConfig())
	cases := map[string]string{
		"utf8mb4_unicode_ci":     "utf8mb4_0900_ai_ci", // configured pattern
		"utf8mb4_unicode_520_ci": "utf8mb4_0900_ai_ci", // configured pattern
		"latin1_swedish_ci":      "utf8mb4_0900_ai_ci", // configured exact
		"latin1_german1_ci":      "utf8mb4_0900_ai_ci", // charset latin1 is mapped
		"utf8mb4_uca1400_ai_ci":  "utf8mb4_0900_ai_ci", // MariaDB 11 default
		"utf8mb4_uca1400_as_cs":  "utf8mb4_bin",        // stays case-sensitive
		"utf8mb4_nopad_bin":      "utf8mb4_0900_bin",   // keeps NO PAD
		"utf8mb3_uca1400_ai_ci":  "utf8mb3_general_ci",
		"utf8mb3_uca1400_as_cs":  "utf8mb3_bin",
		"utf8mb4_bin":            "utf8mb4_bin", // supported: kept
		"utf8mb4_general_ci":     "utf8mb4_general_ci",
		"gbk_chinese_ci":         "gbk_chinese_ci",
		"cp1251_general_ci":      "cp1251_general_ci", // unknown charset: left for the parser to reject
		"latin1_general_cs":      "utf8mb4_bin",       // case-sensitive wins over the mapping's ai_ci
		"latin1_bin":             "utf8mb4_bin",       // supported, but latin1 is mapped to utf8mb4
		"gbk_chinese_nopad_ci":   "gbk_chinese_ci",    // stays case-insensitive
		"ascii_general_ci":       "ascii_bin",         // only binary exists (reported)
	}
	for in, want := range cases {
		got, _ := r.Resolve(in)
		assert.Equal(t, want, got, in)
	}
}

func TestResolveCollationPatternsAreDeterministic(t *testing.T) {
	cfg := &config.Config{CollationMappings: map[string]string{
		"utf8mb4_*":         "utf8mb4_general_ci",
		"utf8mb4_unicode_*": "utf8mb4_unicode_ci",
		"utf8mb4_uca1400_*": "utf8mb4_0900_ai_ci",
	}}
	for range 50 { // map iteration order varies between runs
		r := NewCollationResolver(cfg)
		got, _ := r.Resolve("utf8mb4_unicode_520_ci")
		assert.Equal(t, "utf8mb4_unicode_ci", got)
		got, _ = r.Resolve("utf8mb4_uca1400_ai_ci")
		assert.Equal(t, "utf8mb4_0900_ai_ci", got)
		got, _ = r.Resolve("utf8mb4_swedish_ci")
		assert.Equal(t, "utf8mb4_general_ci", got)
	}
}

func TestFallbackFollowsConfiguredUnicodeChoice(t *testing.T) {
	// Targets older than v7.4 map unicode collations to general_ci; the
	// fallback for unknown utf8mb4 collations must follow that choice.
	cfg := &config.Config{CollationMappings: map[string]string{"utf8mb4_unicode_*": "utf8mb4_general_ci"}}
	got, _ := NewCollationResolver(cfg).Resolve("utf8mb4_uca1400_ai_ci")
	assert.Equal(t, "utf8mb4_general_ci", got)
}

func TestReporterDedupAndNil(t *testing.T) {
	r := NewReporter()
	r.Warnf("a %d", 1)
	r.Warnf("a %d", 1)
	r.Warnf("b")
	assert.Equal(t, []string{"a 1", "b"}, r.Warnings())
	var nilR *Reporter
	nilR.Warnf("ignored")
	assert.Nil(t, nilR.Warnings())
}

func TestCaveats(t *testing.T) {
	assert.Empty(t, Caveats("utf8mb4_general_ci", "utf8mb4_unicode_ci", false))
	assert.Len(t, Caveats("utf8mb4_unicode_ci", "utf8mb4_0900_ai_ci", false), 1, "PAD SPACE -> NO PAD")
	assert.Len(t, Caveats("utf8mb4_uca1400_as_ci", "utf8mb4_0900_ai_ci", true), 3, "fallback, accents, padding")
	assert.Contains(t, Caveats("latin1_swedish_ci", "utf8mb4_0900_ai_ci", false)[0], "language-specific", "also for a configured mapping")
	assert.NotContains(t, Caveats("utf8mb4_uca1400_ai_ci", "utf8mb4_0900_ai_ci", true)[0], "language-specific")
	assert.Len(t, Caveats("utf8mb4_nopad_bin", "utf8mb4_bin", true), 2, "fallback, NO PAD -> PAD SPACE")
	assert.Len(t, Caveats("ascii_general_ci", "ascii_bin", true), 2, "fallback, case")
}

func TestAccentAndInCharset(t *testing.T) {
	assert.True(t, AccentSensitivityLost("latin1_general_ci", "utf8mb4_0900_ai_ci"))
	assert.True(t, AccentSensitivityLost("utf8mb3_uca1400_as_ci", "utf8mb3_general_ci"))
	assert.False(t, AccentSensitivityLost("latin1_swedish_ci", "utf8mb4_0900_ai_ci"))
	assert.Empty(t, Caveats("utf8mb4_nopad_bin", "utf8mb4_0900_bin", true), "equivalent binary collations")

	r := NewCollationResolver(config.DefaultConfig())
	for in, want := range map[string]string{
		"latin1_general_cs":     "latin1_bin", // stays latin1: the expression's charset is not remapped
		"latin1_bin":            "latin1_bin",
		"utf8mb4_uca1400_ai_ci": "utf8mb4_0900_ai_ci",
	} {
		got, _, _ := r.ResolveInCharset(in)
		assert.Equal(t, want, got, in)
	}
}
