package parser

import (
	"strconv"
	"strings"
)

// tokKind classifies a lexical token of a MySQL/MariaDB script.
type tokKind int

const (
	tokSpace       tokKind = iota // whitespace
	tokComment                    // -- , #, /* */ and MariaDB-only /*M! */ comments
	tokExecOpen                   // "/*!" or "/*!NNNNN": the content is code
	tokExecClose                  // "*/" closing an executable comment
	tokWord                       // keyword, identifier, number, @var
	tokQuotedIdent                // `...`
	tokString                     // '...' or "..."
	tokPunct                      // any other single byte
)

// token is a lexical token. Concatenating the text of all tokens of an input
// yields the input unchanged, so token-level rewrites never touch anything
// they do not explicitly replace.
type token struct {
	kind tokKind
	text string
}

// significant reports whether the token carries code (not space, comment or
// executable-comment markers).
func (t token) significant() bool {
	return t.kind != tokSpace && t.kind != tokComment && t.kind != tokExecOpen && t.kind != tokExecClose
}

// is reports whether t is the unquoted word w (case-insensitive).
func (t token) is(w string) bool {
	return t.kind == tokWord && strings.EqualFold(t.text, w)
}

// name returns the identifier a word or quoted identifier token denotes.
func (t token) name() string {
	if t.kind == tokQuotedIdent && len(t.text) >= 2 {
		return strings.ReplaceAll(t.text[1:len(t.text)-1], "``", "`")
	}
	return t.text
}

// tokenize splits s into tokens with backslash escapes in strings (the
// default sql_mode).
func tokenize(s string) []token {
	return tokenizeMode(s, false)
}

// tokenizeMode splits s into tokens; with noBackslash (sql_mode
// NO_BACKSLASH_ESCAPES) a backslash in a string is an ordinary character.
// It never fails: unterminated strings, identifiers and comments extend to
// the end of the input.
func tokenizeMode(s string, noBackslash bool) []token {
	var toks []token
	inExec := false
	i := 0
	for i < len(s) {
		c := s[i]
		start := i
		var kind tokKind
		switch {
		case isSpace(c):
			for i < len(s) && isSpace(s[i]) {
				i++
			}
			kind = tokSpace
		case c == '\'' || c == '"' || c == '`':
			i = skipQuoted(s, i, noBackslash)
			kind = tokString
			if c == '`' {
				kind = tokQuotedIdent
			}
		case c == '#' || isDashComment(s, i):
			i = skipLine(s, i)
			if inExec { // as in skipExecComment: "*/" still ends the block
				if j := strings.Index(s[start:i], "*/"); j >= 0 {
					i = start + j
				}
			}
			kind = tokComment
		case inExec && strings.HasPrefix(s[i:], "*/"):
			i += 2
			inExec = false
			kind = tokExecClose
		case strings.HasPrefix(s[i:], "/*!") && !inExec && neverExecuted(s[i:]):
			i = skipExecComment(s, i, noBackslash)
			kind = tokComment
		case strings.HasPrefix(s[i:], "/*!") && !inExec:
			i += 3
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			inExec = true
			kind = tokExecOpen
		case strings.HasPrefix(s[i:], "/*M!"):
			// MariaDB-only code: one comment token, but its end is found
			// outside strings, as the client does.
			i = skipExecComment(s, i, noBackslash)
			kind = tokComment
		case strings.HasPrefix(s[i:], "/*"):
			i = skipBlockComment(s, i)
			kind = tokComment
		case isWordByte(c):
			for i < len(s) && isWordByte(s[i]) {
				i++
			}
			kind = tokWord
		default:
			i++
			kind = tokPunct
		}
		toks = append(toks, token{kind: kind, text: s[start:i]})
	}
	return toks
}

// joinTokens concatenates token texts back into SQL.
func joinTokens(toks []token) string {
	var b strings.Builder
	for _, t := range toks {
		b.WriteString(t.text)
	}
	return b.String()
}

// isWordByte reports bytes that form unquoted words: ASCII letters, digits,
// '_', '$', '@' (user/system variables) and any non-ASCII byte (identifiers
// may contain UTF-8 characters).
func isWordByte(c byte) bool {
	return isAlphaNum(c) || c == '_' || c == '$' || c == '@' || c >= 0x80
}

// isDashComment reports a "-- " comment: MySQL requires whitespace or the
// end of input after the two dashes.
func isDashComment(s string, i int) bool {
	if !strings.HasPrefix(s[i:], "--") {
		return false
	}
	return i+2 == len(s) || isSpace(s[i+2])
}

// skipQuoted returns the index after the quoted token starting at i. Quotes
// are escaped by doubling; inside strings (not identifiers) a backslash
// escapes the next byte unless noBackslash (NO_BACKSLASH_ESCAPES) is set.
func skipQuoted(s string, i int, noBackslash bool) int {
	q := s[i]
	i++
	for i < len(s) {
		switch {
		case s[i] == '\\' && q != '`' && !noBackslash:
			i += 2
		case s[i] == q:
			if i+1 < len(s) && s[i+1] == q {
				i += 2
				continue
			}
			return i + 1
		default:
			i++
		}
	}
	return len(s)
}

// skipLine returns the index of the newline ending the line at i (kept out of
// the comment so line-based directives still see it), or len(s).
func skipLine(s string, i int) int {
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(s)
}

// neverExecuted reports an executable comment whose version no server
// reaches, /*!999999 ... */: MySQL and MariaDB skip it, so it is a comment.
func neverExecuted(s string) bool {
	d := 0
	for 3+d < len(s) && s[3+d] >= '0' && s[3+d] <= '9' {
		d++
	}
	return d >= 6 && s[3:3+d] >= "999999"
}

// splitQuoted splits a string token into its opening quote, body and
// closing quote; an unterminated string (possibly a lone quote) has no
// closing quote.
func splitQuoted(text string) (open, body, closing string) {
	if text == "" {
		return "", "", ""
	}
	if len(text) >= 2 && text[len(text)-1] == text[0] {
		return text[:1], text[1 : len(text)-1], text[len(text)-1:]
	}
	return text[:1], text[1:], ""
}

// skipExecComment returns the index after the executable comment (/*! or
// /*M!) at i. Its content is code: a "*/" inside a string does not end it.
func skipExecComment(s string, i int, noBackslash bool) int {
	i += 3
	for i < len(s) {
		switch {
		case s[i] == '\'' || s[i] == '"' || s[i] == '`':
			i = skipQuoted(s, i, noBackslash)
		case s[i] == '#' || isDashComment(s, i):
			// An apostrophe in "-- don't" is not a string. The comment
			// still ends at "*/", as in the client.
			eol := skipLine(s, i)
			if j := strings.Index(s[i:eol], "*/"); j >= 0 {
				return i + j + 2
			}
			i = eol
		case strings.HasPrefix(s[i:], "*/"):
			return i + 2
		default:
			i++
		}
	}
	return len(s)
}

// skipBlockComment returns the index after the /* */ comment at i.
func skipBlockComment(s string, i int) int {
	if j := strings.Index(s[i+2:], "*/"); j >= 0 {
		return i + 2 + j + 2
	}
	return len(s)
}

// scriptStmt is one statement of a script and the escaping mode it was
// written in.
type scriptStmt struct {
	text        string
	noBackslash bool // sql_mode had NO_BACKSLASH_ESCAPES
}

// splitScript splits a mysql-client script into statements, honoring the
// client-side DELIMITER directive (e.g. around trigger and routine bodies in
// mariadb-dump output). Delimiters inside strings, quoted identifiers,
// comments and executable comments do not split. Like the client, it
// follows SET sql_mode statements: mariadb-dump writes routines created
// under NO_BACKSLASH_ESCAPES in that mode, where '\' does not escape.
// Returned statements exclude their delimiter; blank ones (only space and
// comments) are kept so that no text is lost, and callers decide what to
// drop.
func splitScript(s string) []string {
	var out []string
	for _, st := range splitScriptModes(s, nil) {
		out = append(out, st.text)
	}
	return out
}

// splitScriptModes is splitScript with each statement's escaping mode; warn
// (may be nil) hears about sql_mode assignments it cannot evaluate.
func splitScriptModes(s string, warn func(format string, args ...any)) []scriptStmt {
	s = strings.TrimPrefix(s, "\ufeff") // UTF-8 BOM, which the client skips
	var stmts []scriptStmt
	mode := sqlModeTracker{warn: warn}
	emit := func(text string) {
		nbe := mode.noBackslash()
		stmts = append(stmts, scriptStmt{text: text, noBackslash: nbe})
		mode.update(tokenizeMode(text, nbe))
	}
	delim := ";"
	start := 0
	atStart := true // only space/comments since the last delimiter
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case atStart && atLineStart(s, i) && hasDelimiterDirective(s, i):
			eol := skipLine(s, i)
			// Like the client, only the first word is the delimiter.
			if f := strings.Fields(s[i+len("delimiter") : eol]); len(f) > 0 {
				delim = f[0]
			}
			if strings.TrimSpace(s[start:i]) != "" {
				emit(s[start:i])
			}
			i = eol
			start = i
		case isSpace(c):
			i++
		case c == '\'' || c == '"' || c == '`':
			i = skipQuoted(s, i, mode.noBackslash())
			atStart = false
		case c == '#' || isDashComment(s, i):
			i = skipLine(s, i)
		case strings.HasPrefix(s[i:], "/*!") || strings.HasPrefix(s[i:], "/*M!"):
			i = skipExecComment(s, i, mode.noBackslash())
			atStart = false
		case strings.HasPrefix(s[i:], "/*"):
			i = skipBlockComment(s, i)
		case strings.HasPrefix(s[i:], delim):
			emit(s[start:i])
			i += len(delim)
			start = i
			atStart = true
		default:
			i++
			atStart = false
		}
	}
	if start < len(s) {
		emit(s[start:])
	}
	return stmts
}

// sqlModeTracker follows sql_mode through SET statements: literal values,
// @@sql_mode, and user variables saved from it (mariadb-dump's
// "SET @saved_sql_mode = @@sql_mode" ... "SET sql_mode = @saved_sql_mode").
type sqlModeTracker struct {
	cur  string // as the server reports it: upper case
	vars map[string]string
	warn func(format string, args ...any)
}

func (m *sqlModeTracker) noBackslash() bool {
	return strings.Contains(m.cur, "NO_BACKSLASH_ESCAPES")
}

func (m *sqlModeTracker) update(toks []token) {
	// MariaDB executes /*M!NNNNN ... */ comments; so must the tracker.
	for _, t := range toks {
		if t.kind == tokComment && strings.HasPrefix(t.text, "/*M!") {
			inner := strings.TrimSuffix(strings.TrimLeft(t.text[4:], "0123456789"), "*/")
			m.update(tokenizeMode(inner, m.noBackslash()))
		}
	}
	sig := sigIndexes(toks)
	if len(sig) == 0 || !toks[sig[0]].is("SET") {
		return
	}
	if m.vars == nil {
		m.vars = map[string]string{}
	}
	var lhs []token
	var rhs []token
	inRHS := false
	depth := 0
	flush := func() {
		if len(lhs) > 0 && len(rhs) > 0 {
			m.assign(lhs, rhs)
		}
		lhs, rhs, inRHS = nil, nil, false
	}
	for _, k := range sig[1:] {
		t := toks[k]
		switch {
		case t.text == "(":
			depth++
		case t.text == ")":
			depth--
		case depth == 0 && t.text == ",":
			flush()
			continue
		case depth == 0 && !inRHS && (t.text == "=" || t.text == ":"):
			inRHS = true
			continue
		case inRHS && len(rhs) == 0 && t.text == "=": // the "=" of ":="
			continue
		}
		if inRHS {
			rhs = append(rhs, t)
		} else {
			lhs = append(lhs, t)
		}
	}
	flush()
}

func (m *sqlModeTracker) assign(lhs, rhs []token) {
	var name string
	for _, t := range lhs {
		name += t.name() // `sql_mode` counts like sql_mode
	}
	name = strings.ToLower(name)
	isVar := strings.HasPrefix(name, "@") && !strings.HasPrefix(name, "@@")
	isMode := strings.HasSuffix(name, "sql_mode") && !strings.Contains(name, "global")
	if !isVar && !isMode {
		return
	}
	value, ok := m.eval(rhs)
	switch {
	case !ok && isMode:
		// Keep the current mode, and say so: if the value switched
		// NO_BACKSLASH_ESCAPES, strings that follow are read wrongly.
		if m.warn != nil {
			var text string
			for _, t := range rhs {
				text += t.text
			}
			m.warn("cannot evaluate SET sql_mode = %s; assuming NO_BACKSLASH_ESCAPES is unchanged (%v), check backslashes in the strings that follow", text, m.noBackslash())
		}
	case !ok:
		delete(m.vars, name)
	case isVar:
		m.vars[name] = value
	default:
		m.cur = strings.ToUpper(decodeSQLModeNumber(value)) // the server canonicalizes the mode
	}
}

// eval evaluates a sql_mode value: a string literal, @@sql_mode,
// @@session.sql_mode, a user variable, DEFAULT, and CONCAT / REPLACE over
// those. It reports false for anything else rather than guessing from the
// text (REPLACE(@@sql_mode, 'NO_BACKSLASH_ESCAPES', ”) contains the flag
// but turns it off).
func (m *sqlModeTracker) eval(rhs []token) (string, bool) {
	v, rest, ok := m.evalExpr(rhs)
	return v, ok && len(rest) == 0
}

func (m *sqlModeTracker) evalExpr(toks []token) (string, []token, bool) {
	if len(toks) == 0 {
		return "", nil, false
	}
	t := toks[0]
	switch {
	case t.kind == tokString:
		// Case kept: REPLACE is case-sensitive, so REPLACE(@@sql_mode,
		// 'no_backslash_escapes', '') leaves the (upper-case) flag on.
		_, body, _ := splitQuoted(t.text)
		return body, toks[1:], true
	case t.is("DEFAULT"):
		return "", toks[1:], true
	case t.text == "(":
		v, rest, ok := m.evalExpr(toks[1:])
		if !ok || len(rest) == 0 || rest[0].text != ")" {
			return "", nil, false
		}
		return v, rest[1:], true
	case t.is("CONCAT") || t.is("CONCAT_WS") || t.is("REPLACE") || t.is("UPPER") || t.is("LOWER") || t.is("IF"):
		args, rest, ok := m.evalArgs(toks[1:])
		if !ok {
			return "", nil, false
		}
		switch {
		case t.is("CONCAT"):
			return strings.Join(args, ""), rest, true
		case t.is("CONCAT_WS") && len(args) >= 1:
			return strings.Join(args[1:], args[0]), rest, true
		case t.is("REPLACE") && len(args) == 3:
			return strings.ReplaceAll(args[0], args[1], args[2]), rest, true
		case t.is("UPPER") && len(args) == 1:
			return strings.ToUpper(args[0]), rest, true
		case t.is("LOWER") && len(args) == 1:
			return strings.ToLower(args[0]), rest, true
		case t.is("IF") && len(args) == 3:
			// Only a constant condition: a number literal.
			if n, err := strconv.ParseFloat(args[0], 64); err == nil {
				if n != 0 {
					return args[1], rest, true
				}
				return args[2], rest, true
			}
		}
		return "", nil, false
	case t.kind == tokWord && t.text[0] >= '0' && t.text[0] <= '9':
		return t.text, toks[1:], true // a number, e.g. IF's condition
	case strings.HasPrefix(t.text, "@@"):
		// @@sql_mode, or @@SESSION . sql_mode / @@LOCAL . sql_mode
		n := 1
		if len(toks) > 2 && toks[1].text == "." {
			n = 3
		}
		if strings.HasSuffix(strings.ToLower(toks[n-1].text), "sql_mode") {
			return m.cur, toks[n:], true
		}
	case strings.HasPrefix(t.text, "@"):
		v, ok := m.vars[strings.ToLower(t.text)]
		return v, toks[1:], ok // MariaDB rejects an undefined variable
	}
	return "", nil, false
}

// noBackslashEscapesBit is MODE_NO_BACKSLASH_ESCAPES in MariaDB's (and
// MySQL's) sql_mode bitmask: SET sql_mode = 1048576 enables it.
const noBackslashEscapesBit = 1 << 20

// decodeSQLModeNumber turns a numeric sql_mode into the flag names the
// tracker needs; other values are returned unchanged.
func decodeSQLModeNumber(v string) string {
	n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return v
	}
	if n&noBackslashEscapesBit != 0 {
		return "NO_BACKSLASH_ESCAPES"
	}
	return ""
}

// evalArgs evaluates "( expr, expr, ... )".
func (m *sqlModeTracker) evalArgs(toks []token) ([]string, []token, bool) {
	if len(toks) == 0 || toks[0].text != "(" {
		return nil, nil, false
	}
	var args []string
	rest := toks[1:]
	for {
		v, r, ok := m.evalExpr(rest)
		if !ok || len(r) == 0 {
			return nil, nil, false
		}
		args = append(args, v)
		switch r[0].text {
		case ",":
			rest = r[1:]
		case ")":
			return args, r[1:], true
		default:
			return nil, nil, false
		}
	}
}

// SplitScript splits a mysql-client script into statements without their
// delimiters, honoring DELIMITER directives; blank statements are omitted.
func SplitScript(s string) []string {
	var out []string
	for _, stmt := range splitScript(s) {
		if strings.TrimSpace(stmt) != "" {
			out = append(out, stmt)
		}
	}
	return out
}

// atLineStart reports whether only spaces/tabs precede i on its line.
func atLineStart(s string, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch s[j] {
		case '\n':
			return true
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return true
}

// hasDelimiterDirective reports a "DELIMITER <x>" directive at i.
func hasDelimiterDirective(s string, i int) bool {
	const kw = "delimiter"
	if len(s)-i <= len(kw) || !strings.EqualFold(s[i:i+len(kw)], kw) {
		return false
	}
	return s[i+len(kw)] == ' ' || s[i+len(kw)] == '\t'
}
