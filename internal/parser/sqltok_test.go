package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenizeIsLossless(t *testing.T) {
	inputs := []string{
		"",
		"SELECT 1",
		"INSERT INTO t VALUES ('it''s', \"q\\\"x\", `id``x`) -- c\n# h\n/* b */ /*!40101 SET x=1 */ /*M!100 y */;",
		"'unterminated",
		"/*! unterminated exec",
		"/* unterminated",
		"`unterminated",
		"--\n--x",
		"caf\xc3\xa9 \x80\xff",
	}
	for _, in := range inputs {
		assert.Equal(t, in, joinTokens(tokenize(in)), "input %q", in)
	}
}

func TestTokenizeKinds(t *testing.T) {
	toks := tokenize("a 'b' `c` /*!50003 d */ /*M! e */ -- f\n--g")
	var kinds []tokKind
	for _, tk := range toks {
		if tk.kind != tokSpace {
			kinds = append(kinds, tk.kind)
		}
	}
	// "--g" is not a comment: MySQL needs whitespace after the dashes.
	assert.Equal(t, []tokKind{tokWord, tokString, tokQuotedIdent, tokExecOpen, tokWord, tokExecClose, tokComment, tokComment, tokPunct, tokPunct, tokWord}, kinds)
}

func TestSplitScriptDelimiters(t *testing.T) {
	script := "SET a=1;\n" +
		"DELIMITER ;;\n" +
		"/*!50003 CREATE*/ /*!50003 TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET NEW.a = 1; SET NEW.b = ';'; END */;;\n" +
		"DELIMITER ;\n" +
		"INSERT INTO t VALUES ('a;b', \"c;d\", `e;f`); -- not; a split\n" +
		"SELECT /* x; y */ 2"
	var got []string
	for _, s := range splitScript(script) {
		if strings.TrimSpace(s) != "" {
			got = append(got, strings.TrimSpace(s))
		}
	}
	assert.Equal(t, []string{
		"SET a=1",
		"/*!50003 CREATE*/ /*!50003 TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET NEW.a = 1; SET NEW.b = ';'; END */",
		"INSERT INTO t VALUES ('a;b', \"c;d\", `e;f`)",
		"-- not; a split\nSELECT /* x; y */ 2",
	}, got)
}

func TestSplitScriptDelimiterOnlyAtStatementStart(t *testing.T) {
	// "delimiter" inside a statement is an identifier, not a directive.
	got := splitScript("SELECT 1 AS\ndelimiter ;\nSELECT 2;")
	assert.Equal(t, "SELECT 1 AS\ndelimiter ", got[0])
}

func FuzzTokenize(f *testing.F) {
	for _, s := range []string{"SELECT 'a''b' `c` /*!1 x */ -- y\n", "/*", "'", "DELIMITER ;;\nx;;\n"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got := joinTokens(tokenize(s)); got != s {
			t.Fatalf("tokenize is lossy: %q -> %q", s, got)
		}
		_ = splitScript(s)
	})
}

func TestLineCommentInsideExecutableComment(t *testing.T) {
	in := "/*!50003 SET a = 1 -- don't */ SELECT 2"
	toks := tokenize(in)
	assert.Equal(t, in, joinTokens(toks))
	var kinds []tokKind
	for _, tk := range toks {
		if tk.kind != tokSpace {
			kinds = append(kinds, tk.kind)
		}
	}
	assert.Equal(t, []tokKind{tokExecOpen, tokWord, tokWord, tokPunct, tokWord, tokComment, tokExecClose, tokWord, tokWord}, kinds)
	assert.Equal(t, []string{"/*!50003 SET a = 1 -- don't */ SELECT 2"}, SplitScript(in))
}
