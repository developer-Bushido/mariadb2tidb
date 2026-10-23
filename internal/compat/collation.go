// Package compat holds TiDB compatibility facts shared by the loader and the
// transformation rules: which collations and engines TiDB accepts, how
// MariaDB-only names map onto them, and a collector for migration warnings.
package compat

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/config"
)

// supportedCollations is information_schema.collations of TiDB v8.5 with the
// new collation framework (the default since v6.0). The parser's own
// charset.GetSupportedCollations() lists only the legacy subset, and its
// GetCollationByName accepts MySQL names the server rejects with error 1273
// (e.g. latin1_swedish_ci, utf8mb4_unicode_520_ci).
var supportedCollations = map[string]bool{
	"ascii_bin": true, "binary": true, "gbk_bin": true, "gbk_chinese_ci": true,
	"latin1_bin": true, "utf8_bin": true, "utf8_general_ci": true, "utf8_unicode_ci": true,
	"utf8mb3_bin": true, "utf8mb3_general_ci": true, "utf8mb3_unicode_ci": true,
	"utf8mb4_0900_ai_ci": true, "utf8mb4_0900_bin": true, "utf8mb4_bin": true,
	"utf8mb4_general_ci": true, "utf8mb4_unicode_ci": true,
}

// supportedCharsets is SHOW CHARACTER SET of TiDB v8.5 (utf8mb3 is an alias
// of utf8).
var supportedCharsets = map[string]bool{
	"ascii": true, "binary": true, "gbk": true, "latin1": true,
	"utf8": true, "utf8mb3": true, "utf8mb4": true,
}

// CharsetSupported reports whether TiDB accepts the charset name.
func CharsetSupported(name string) bool {
	return supportedCharsets[strings.ToLower(name)]
}

// CollationSupported reports whether TiDB accepts the collation name.
func CollationSupported(name string) bool {
	return supportedCollations[strings.ToLower(name)]
}

// CollationResolver maps source collations onto TiDB ones: configured exact
// names first, then configured wildcard patterns (most specific first, so
// overlapping patterns resolve deterministically), then a fallback by
// charset for names TiDB does not support.
type CollationResolver struct {
	exact    map[string]string
	patterns []collationPattern
	charsets map[string]config.CharsetMapping
}

type collationPattern struct {
	glob, target string
}

// NewCollationResolver builds a resolver from cfg's collation_mappings and
// charset_mappings. A nil cfg yields the built-in fallback only.
func NewCollationResolver(cfg *config.Config) *CollationResolver {
	r := &CollationResolver{
		exact:    map[string]string{},
		charsets: map[string]config.CharsetMapping{},
	}
	if cfg == nil {
		return r
	}
	for src, dst := range cfg.CollationMappings {
		src = strings.ToLower(src)
		if strings.ContainsAny(src, "*?[") {
			r.patterns = append(r.patterns, collationPattern{glob: src, target: dst})
		} else {
			r.exact[src] = dst
		}
	}
	// More literal characters = more specific; ties broken lexically.
	sort.Slice(r.patterns, func(i, j int) bool {
		li, lj := literalLen(r.patterns[i].glob), literalLen(r.patterns[j].glob)
		if li != lj {
			return li > lj
		}
		return r.patterns[i].glob < r.patterns[j].glob
	})
	for cs, m := range cfg.CharsetMappings {
		r.charsets[strings.ToLower(cs)] = m
	}
	return r
}

func literalLen(glob string) int {
	n := 0
	for _, c := range glob {
		if c != '*' && c != '?' {
			n++
		}
	}
	return n
}

// Resolve returns the TiDB collation for name and whether it differs from
// name. Supported names are kept unless explicitly mapped or their charset
// is mapped to another one (latin1_bin under latin1 -> utf8mb4 becomes
// utf8mb4_bin; keeping it would be rejected with error 1253).
func (r *CollationResolver) Resolve(name string) (string, bool) {
	dst, changed, _ := r.ResolveDetail(name)
	return dst, changed
}

// ResolveDetail is Resolve that also reports whether the result is a
// fallback (no configured mapping matched), whose comparison rules are the
// closest TiDB has rather than a choice the user made.
func (r *CollationResolver) ResolveDetail(name string) (dst string, changed, fallback bool) {
	lower := strings.ToLower(name)
	if dst, ok := r.exact[lower]; ok {
		return dst, !strings.EqualFold(dst, name), false
	}
	for _, p := range r.patterns {
		if ok, _ := filepath.Match(p.glob, lower); ok {
			return p.target, !strings.EqualFold(p.target, name), false
		}
	}
	if CollationSupported(lower) && !r.charsetRemapped(CollationCharset(lower)) {
		return name, false, false
	}
	if dst := r.fallback(lower); dst != "" {
		return dst, !strings.EqualFold(dst, name), true
	}
	return name, false, false
}

func (r *CollationResolver) charsetRemapped(cs string) bool {
	m, ok := r.charsets[cs]
	return ok && m.TargetCharset != "" && !strings.EqualFold(m.TargetCharset, cs)
}

// CaseSensitive reports whether a collation name compares case-sensitively
// (binary, or an explicit _cs suffix).
func CaseSensitive(name string) bool {
	name = strings.ToLower(name)
	return name == "binary" || strings.HasSuffix(name, "_bin") || strings.HasSuffix(name, "_cs")
}

// fallback picks a supported collation in the (possibly mapped) charset
// with the same case sensitivity: sensitivity is decided first, so a *_cs
// or *_bin collation never lands on a case-insensitive one (unique keys
// would reject rows differing only in case) and vice versa where the
// target charset has a case-insensitive collation.
func (r *CollationResolver) fallback(name string) string {
	cs := CollationCharset(name)
	if m, ok := r.charsets[cs]; ok && m.TargetCharset != "" {
		if !CaseSensitive(name) && m.TargetCollation != "" && !CaseSensitive(m.TargetCollation) {
			return m.TargetCollation
		}
		cs = strings.ToLower(m.TargetCharset)
	}
	return r.fallbackIn(cs, name)
}

// ResolveInCharset resolves a collation that must stay in its own charset:
// a COLLATE applied to an expression with an explicit charset, such as
// CONVERT(x USING latin1) COLLATE latin1_general_cs, whose charset the
// conversion leaves alone. Supported names are kept.
func (r *CollationResolver) ResolveInCharset(name string) (dst string, changed, fallback bool) {
	if CollationSupported(name) {
		return name, false, false
	}
	if dst := r.fallbackIn(CollationCharset(name), strings.ToLower(name)); dst != "" {
		return dst, !strings.EqualFold(dst, name), true
	}
	return name, false, false
}

// fallbackIn picks a supported collation of charset cs with the case
// sensitivity of name.
func (r *CollationResolver) fallbackIn(cs, name string) string {
	sensitive := CaseSensitive(name)
	switch cs {
	case "utf8mb4":
		if sensitive && strings.Contains(name, "nopad") {
			return "utf8mb4_0900_bin" // also NO PAD
		}
		if sensitive {
			return "utf8mb4_bin"
		}
		// Follow the configured choice for utf8mb4_unicode_ci so targets
		// older than v7.4 (no utf8mb4_0900_ai_ci) stay consistent.
		if dst, _ := r.Resolve("utf8mb4_unicode_ci"); CollationSupported(dst) && !CaseSensitive(dst) {
			return dst
		}
		return "utf8mb4_0900_ai_ci"
	case "utf8mb3", "utf8":
		if sensitive {
			return "utf8mb3_bin"
		}
		return "utf8mb3_general_ci"
	case "gbk":
		if sensitive {
			return "gbk_bin"
		}
		return "gbk_chinese_ci"
	case "latin1", "ascii":
		// TiDB has only the binary collation for these charsets.
		return cs + "_bin"
	}
	return ""
}

// noPadCollations are the TiDB collations with PAD_ATTRIBUTE NO PAD
// (information_schema.collations of v8.5): trailing spaces are significant.
var noPadCollations = map[string]bool{"binary": true, "utf8mb4_0900_ai_ci": true, "utf8mb4_0900_bin": true}

// padSpace reports whether a collation ignores trailing spaces. MariaDB's
// collations are PAD SPACE unless named *_nopad_*; MySQL's *_0900_* and
// binary are NO PAD.
func padSpace(name string) bool {
	name = strings.ToLower(name)
	return !noPadCollations[name] && !strings.Contains(name, "nopad") && !strings.Contains(name, "_0900_")
}

// Caveats describes how comparisons change when a column's collation goes
// from one name to another; fallback marks a mapping no config chose.
func Caveats(from, to string, fallback bool) []string {
	var out []string
	// Binary collations with the same padding compare the same bytes:
	// utf8mb4_nopad_bin and utf8mb4_0900_bin are equivalent.
	if strings.HasSuffix(strings.ToLower(from), "_bin") && strings.HasSuffix(strings.ToLower(to), "_bin") &&
		padSpace(from) == padSpace(to) && CollationCharset(from) == CollationCharset(to) {
		return nil
	}
	switch {
	case languageSpecific(from) && !languageSpecific(to):
		out = append(out, "its language-specific equality and order rules are lost (e.g. å, ß, ð)")
	case fallback:
		out = append(out, "no exact TiDB equivalent, closest one used")
	}
	if AccentSensitivityLost(from, to) {
		out = append(out, "accent-sensitive values become equal (e = é), unique keys may reject rows")
	}
	if SensitivityChanged(from, to) {
		out = append(out, "comparisons become case-sensitive (TiDB has no case-insensitive collation for this charset)")
	}
	switch {
	case padSpace(from) && !padSpace(to):
		out = append(out, "trailing spaces become significant (PAD SPACE -> NO PAD: 'a' and 'a ' are no longer equal)")
	case !padSpace(from) && padSpace(to):
		out = append(out, "trailing spaces are ignored (NO PAD -> PAD SPACE: 'a' and 'a ' become equal, unique keys may reject rows)")
	}
	return out
}

// languageSpecific reports collations with language rules, such as
// latin1_swedish_ci or utf8mb4_german2_ci, as opposed to generic ones
// (general, unicode, uca1400, 0900, bin).
func languageSpecific(name string) bool {
	generic := map[string]bool{
		"general": true, "unicode": true, "uca1400": true, "0900": true, "520": true, "bin": true,
		"nopad": true, "ai": true, "as": true, "ci": true, "cs": true, "mysql500": true,
	}
	parts := strings.Split(strings.ToLower(name), "_")
	for _, p := range parts[1:] {
		if !generic[p] {
			return true
		}
	}
	return false
}

// SensitivityChanged reports a mapping that makes case-insensitive
// comparisons case-sensitive (only possible for charsets TiDB supports
// with a binary collation alone, e.g. ascii, latin1 without a mapping).
func SensitivityChanged(from, to string) bool {
	return !CaseSensitive(from) && CaseSensitive(to)
}

// CollationCharset returns the charset prefix of a collation name.
func CollationCharset(name string) string {
	name = strings.ToLower(name)
	for _, cs := range []string{"utf8mb4", "utf8mb3"} {
		if strings.HasPrefix(name, cs+"_") {
			return cs
		}
	}
	if i := strings.IndexByte(name, '_'); i > 0 {
		return name[:i]
	}
	return name
}

// AccentSensitivityLost reports a mapping from an accent-sensitive,
// case-insensitive collation to an accent-insensitive one: values that were
// distinct (e.g. 'e' and 'é') become equal, so unique keys may reject rows
// on import.
func AccentSensitivityLost(from, to string) bool {
	return AccentSensitive(from) && strings.HasSuffix(strings.ToLower(to), "_ci")
}

// AccentSensitive reports a case-insensitive collation that still tells
// accents apart: MariaDB's *_as_* collations and latin1_general_ci
// (checked on MariaDB 11.4: 'e' = 'é' is false only for these). Every
// TiDB *_ci collation is accent-insensitive.
func AccentSensitive(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "_as_") || name == "latin1_general_ci"
}
