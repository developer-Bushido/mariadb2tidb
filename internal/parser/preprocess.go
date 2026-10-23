package parser

import (
	"strconv"
	"strings"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
)

// preprocessor rewrites MariaDB-only syntax the TiDB parser cannot read.
// It works on tokens, so string literals, comments and quoted identifiers
// are never rewritten, and it only touches code in the statement kinds it
// understands.
type preprocessor struct {
	collations *compat.CollationResolver
	charsets   map[string]charsetTarget // source charset -> target
	report     *compat.Reporter
}

// charsetTarget is the configured replacement for a source charset.
type charsetTarget struct {
	charset, collation string
}

// stmtKind classifies a statement by its leading keywords.
type stmtKind int

const (
	stmtOther stmtKind = iota
	stmtSet
	stmtCreateTable
	stmtAlterTable
	stmtCreateSequence
	stmtSetval
	stmtDML         // INSERT / REPLACE / UPDATE / DELETE / SELECT / ...
	stmtProgram     // CREATE TRIGGER / PROCEDURE / FUNCTION / EVENT / PACKAGE
	stmtDropProgram // DROP or ALTER of the above
)

// mapScope selects what mapCharsetsAndCollations may rewrite.
type mapScope int

const (
	mapDDL mapScope = iota // charsets and collations, also as string literals
	mapSet                 // collations, including collation_* variables
	mapDML                 // only COLLATE <name>: never strings or charsets
)

// Column types TiDB does not have, with a data-compatible replacement:
// mariadb-dump writes UUID/INET values as text and spatial values as binary.
var typeReplacements = map[string]string{
	"uuid":               "char(36)",
	"inet4":              "varchar(15)",
	"inet6":              "varchar(39)",
	"geometry":           "longblob",
	"point":              "longblob",
	"linestring":         "longblob",
	"polygon":            "longblob",
	"multipoint":         "longblob",
	"multilinestring":    "longblob",
	"multipolygon":       "longblob",
	"geometrycollection": "longblob",
}

// MariaDB table options the TiDB parser rejects; they only tune storage.
var droppedTableOptions = map[string]bool{
	"PAGE_COMPRESSED": true, "PAGE_COMPRESSION_LEVEL": true, "TRANSACTIONAL": true,
	"PAGE_CHECKSUM": true, "IETF_QUOTES": true, "ENCRYPTED": true, "ENCRYPTION_KEY_ID": true,
}

// Leading words of index/constraint items in a CREATE TABLE body.
var keyItemWords = map[string]bool{
	"PRIMARY": true, "KEY": true, "INDEX": true, "UNIQUE": true, "CONSTRAINT": true,
	"FOREIGN": true, "FULLTEXT": true, "CHECK": true, "SPATIAL": true, "PERIOD": true,
}

// seqState remembers a CREATE SEQUENCE so a following SETVAL can be folded
// into its START WITH value.
type seqState struct {
	schema   string // lower-case database, or the USE database
	name     string
	toks     []token
	out      int // index of the statement in the preprocessor output
	nameIdx  int // index of the sequence name token
	startIdx int // index of the START value token, -1 if absent
	startNeg int // index of a '-' before it, -1 if none
	inc      int64
	min, max int64
	cycle    bool
}

// setStart makes the sequence start at v.
func (st *seqState) setStart(v int64) {
	text := strconv.FormatInt(v, 10)
	if st.startIdx < 0 {
		st.toks[st.nameIdx].text += " START WITH " + text
		return
	}
	if st.startNeg >= 0 {
		blank(st.toks, st.startNeg, st.startNeg+1)
	}
	st.toks[st.startIdx] = token{kind: tokWord, text: text}
}

func (p *preprocessor) run(sql string) string {
	var out [][]token
	var suffix []string // extra statements emitted after out[i]
	seqs := map[string]*seqState{}
	db := "" // current database, from USE

	for _, st := range splitScriptModes(sql, p.report.Warnf) {
		toks := tokenizeMode(st.text, st.noBackslash)
		sig := sigIndexes(toks)
		if len(sig) == 0 {
			continue
		}
		kind, obj := classify(toks, sig)
		switch kind {
		case stmtProgram:
			p.report.Warnf("dropped %s: TiDB does not support stored programs (triggers, procedures, functions, events)", obj)
			continue
		case stmtDropProgram:
			continue
		}
		stripMariaDBComments(toks)
		dropCommentVersions(toks)
		if st.noBackslash {
			normalizeBackslashes(toks)
		}
		if toks[sig[0]].is("USE") && len(sig) > 1 {
			db = strings.ToLower(toks[sig[1]].name())
		}
		switch kind {
		case stmtSet:
			// SET NAMES / CHARACTER SET describe the dump's own bytes: keep
			// the charset, only make collations loadable.
			toks = p.mapCharsetsAndCollations(toks, mapSet)
			stripNoBackslashEscapes(toks)
		case stmtDML:
			toks = p.mapCharsetsAndCollations(toks, mapDML)
		case stmtCreateTable:
			// CREATE TABLE ... AS SELECT: the query part is data.
			q := ctasStart(toks)
			p.mapCharsetsAndCollations(toks[:q], mapDDL)
			if q < len(toks) {
				table, _ := tableName(toks)
				p.report.Warnf("table %s: CREATE TABLE ... SELECT is not supported by TiDB (error 1105); create the table, then INSERT ... SELECT", table)
				p.mapCharsetsAndCollations(toks[q:], mapDML)
			}
		default:
			toks = p.mapCharsetsAndCollations(toks, mapDDL)
		}
		switch kind {
		case stmtCreateTable:
			toks = p.rewriteCreateTable(toks)
		case stmtAlterTable:
			toks = p.rewriteAlterTable(toks)
		case stmtCreateSequence:
			if name, sq := parseSequence(toks); name != "" {
				sq.out = len(out)
				if sq.schema == "" {
					sq.schema = db
				}
				seqs[sq.schema+"."+strings.ToLower(name)] = sq
				out = append(out, sq.toks)
				suffix = append(suffix, "")
				continue
			}
		case stmtSetval:
			var keep bool
			var exhaust *seqState
			toks, keep, exhaust = p.rewriteSetval(toks, seqs, db)
			if exhaust != nil {
				suffix[exhaust.out] = ";\nDO NEXTVAL(" + exhaust.name + ")"
			}
			if !keep {
				continue
			}
		}
		out = append(out, toks)
		suffix = append(suffix, "")
	}

	if len(out) == 0 {
		return ""
	}
	parts := make([]string, len(out))
	for i, t := range out {
		parts[i] = joinTokens(t) + suffix[i]
	}
	return strings.Join(parts, ";\n") + ";\n"
}

// assignsSQLMode reports whether the value token at i is assigned to
// sql_mode ("sql_mode =", "sql_mode :=", "`sql_mode` =").
func assignsSQLMode(toks []token, i int) bool {
	p := prevSig(toks, i)
	if p >= 0 && toks[p].text == "=" {
		p = prevSig(toks, p)
		if p >= 0 && toks[p].text == ":" {
			p = prevSig(toks, p)
		}
	} else {
		return false
	}
	return p >= 0 && strings.HasSuffix(strings.ToLower(toks[p].name()), "sql_mode")
}

// normalizeBackslashes rewrites string literals of a statement written
// under NO_BACKSLASH_ESCAPES ('C:\' is one backslash) into the default
// escaping the TiDB parser and the output use ('C:\\').
func normalizeBackslashes(toks []token) {
	for i, t := range toks {
		if t.kind == tokString && strings.Contains(t.text, "\\") {
			open, body, shut := splitQuoted(t.text)
			toks[i].text = open + strings.ReplaceAll(body, "\\", "\\\\") + shut
		}
	}
}

// stripNoBackslashEscapes removes NO_BACKSLASH_ESCAPES from sql_mode
// values, as a name in string literals and as bit 20 of a numeric value
// (SET sql_mode = 1048576): the output always escapes backslashes, so
// loading it in that mode would double every backslash in the data.
func stripNoBackslashEscapes(toks []token) {
	for i, t := range toks {
		if t.kind == tokWord && t.text[0] >= '0' && t.text[0] <= '9' && assignsSQLMode(toks, i) {
			if n, err := strconv.ParseUint(t.text, 10, 64); err == nil && n&noBackslashEscapesBit != 0 {
				toks[i].text = strconv.FormatUint(n&^noBackslashEscapesBit, 10)
			}
			continue
		}
		if t.kind != tokString || !strings.Contains(strings.ToUpper(t.text), "NO_BACKSLASH_ESCAPES") {
			continue
		}
		open, body, shut := splitQuoted(t.text)
		var keep []string
		for _, m := range strings.Split(body, ",") {
			if !strings.EqualFold(strings.TrimSpace(m), "NO_BACKSLASH_ESCAPES") {
				keep = append(keep, m)
			}
		}
		toks[i].text = open + strings.Join(keep, ",") + shut
	}
}

// stripMariaDBComments blanks /*M! ... */ comments: MariaDB-only code (for
// example the "enable the sandbox mode" line of mariadb-dump 11.x) that
// must not run on TiDB.
func stripMariaDBComments(toks []token) {
	for i, t := range toks {
		if t.kind == tokComment && strings.HasPrefix(t.text, "/*M!") {
			toks[i] = token{kind: tokSpace, text: " "}
		}
	}
}

// dropCommentVersions rewrites six-digit executable-comment versions
// (/*!100616 ...) to a bare /*!, which TiDB executes: its parser reads only
// five digits and fails on the sixth.
func dropCommentVersions(toks []token) {
	for i, t := range toks {
		if t.kind == tokExecOpen && len(t.text) > 3+5 {
			toks[i].text = "/*!"
		}
	}
}

// sigIndexes returns the indexes of significant tokens.
func sigIndexes(toks []token) []int {
	var idx []int
	for i, t := range toks {
		if t.significant() {
			idx = append(idx, i)
		}
	}
	return idx
}

// classify returns the statement kind and, for stored programs, a
// description such as "TRIGGER trg_name".
func classify(toks []token, sig []int) (stmtKind, string) {
	at := func(k int) token {
		if k < len(sig) {
			return toks[sig[k]]
		}
		return token{}
	}
	switch {
	case at(0).is("SET"):
		return stmtSet, ""
	case at(0).is("DO") || at(0).is("SELECT"):
		if at(1).is("SETVAL") && at(2).text == "(" {
			return stmtSetval, ""
		}
		return stmtDML, ""
	case at(0).is("INSERT") || at(0).is("REPLACE") || at(0).is("UPDATE") || at(0).is("DELETE") ||
		at(0).is("WITH") || at(0).is("CALL") || at(0).is("LOAD") || at(0).text == "(":
		return stmtDML, ""
	case at(0).is("ALTER"):
		if at(1).is("TABLE") || (at(1).is("ONLINE") || at(1).is("IGNORE")) && at(2).is("TABLE") {
			return stmtAlterTable, ""
		}
		if isProgramWord(at(1)) {
			return stmtDropProgram, ""
		}
	case at(0).is("DROP"):
		if isProgramWord(at(1)) {
			return stmtDropProgram, ""
		}
	case at(0).is("CREATE"):
		k := 1
		if at(k).is("OR") && at(k+1).is("REPLACE") {
			k += 2
		}
		if at(k).is("TEMPORARY") || at(k).is("GLOBAL") && at(k+1).is("TEMPORARY") {
			if at(k).is("GLOBAL") {
				k++
			}
			k++
		}
		if at(k).is("TABLE") {
			return stmtCreateTable, ""
		}
		if at(k).is("SEQUENCE") {
			return stmtCreateSequence, ""
		}
		// Skip DEFINER = user[@host], SQL SECURITY x, AGGREGATE.
		for {
			switch {
			case at(k).is("DEFINER") && at(k+1).text == "=":
				// user, user@host, 'u'@'h', `u`@`h`, root@'%', CURRENT_USER[()]
				user := at(k + 2)
				k += 3
				if at(k).text == "(" && at(k+1).text == ")" {
					k += 2
				}
				switch {
				case strings.HasSuffix(user.text, "@"):
					k++ // root@ '%'
				case strings.HasPrefix(at(k).text, "@"):
					if at(k).text == "@" {
						k++
					}
					k++
				}
				continue
			case at(k).is("SQL") && at(k+1).is("SECURITY"):
				k += 3
				continue
			case at(k).is("AGGREGATE"):
				k++
				continue
			case at(k).is("ALGORITHM") && at(k+1).text == "=":
				k += 3
				continue
			}
			break
		}
		if at(k).is("VIEW") {
			return stmtDML, "" // a query: its strings are data
		}
		if isProgramWord(at(k)) {
			what := strings.ToUpper(at(k).text)
			n := k + 1
			if at(n).is("BODY") {
				n++
			}
			if at(n).is("IF") {
				n += 3 // IF NOT EXISTS
			}
			return stmtProgram, what + " " + at(n).name()
		}
	}
	return stmtOther, ""
}

func isProgramWord(t token) bool {
	for _, w := range []string{"TRIGGER", "PROCEDURE", "FUNCTION", "EVENT", "PACKAGE"} {
		if t.is(w) {
			return true
		}
	}
	return false
}

// nextSig returns the index of the next significant token after i, or -1.
func nextSig(toks []token, i int) int {
	for j := i + 1; j < len(toks); j++ {
		if toks[j].significant() {
			return j
		}
	}
	return -1
}

// mapCharsetsAndCollations rewrites names after COLLATE / CHARSET /
// CHARACTER SET (optionally followed by '=') so the parser never sees a
// collation TiDB does not know. A mapped charset without its own COLLATE
// gets the mapping's target collation; otherwise the table would silently
// fall back to TiDB's default utf8mb4_bin (case-sensitive). scope limits
// the rewrite: in DML a string after "charset =" is data, and a column may
// be called collation_server.
func (p *preprocessor) mapCharsetsAndCollations(toks []token, scope mapScope) []token {
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != tokWord && t.kind != tokQuotedIdent {
			continue
		}
		var isCollate bool
		var j int
		switch {
		case t.is("COLLATE"), scope == mapSet && isCollationVariable(t):
			isCollate, j = true, nextSig(toks, i)
		case t.is("USING") && insideCall(toks, i, "CONVERT"):
			j = nextSig(toks, i) // CONVERT(x USING latin1)
		case t.kind == tokWord && len(t.text) > 1 && t.text[0] == '_' && t.text[1] != '_':
			p.mapIntroducer(toks, i)
			continue
		case t.is("CHARSET") && (scope == mapDDL || insideCall(toks, i, "CAST", "CONVERT")):
			j = nextSig(toks, i)
		case t.is("CHARACTER") && (scope == mapDDL || insideCall(toks, i, "CAST", "CONVERT")):
			if k := nextSig(toks, i); k > 0 && toks[k].is("SET") {
				j = nextSig(toks, k)
			} else {
				continue
			}
		default:
			continue
		}
		if p.isColumnName(toks, i) {
			continue // a column named charset / character
		}
		inExpr := !t.is("COLLATE") && insideCall(toks, i, "CAST", "CONVERT")
		if !t.is("COLLATE") && !isCollate && !inExpr && parenDepth(toks, i) > 0 {
			if n := nextSig(toks, i); n > 0 && toks[n].text == "=" {
				continue // (charset = 'latin1'): a comparison with a column
			}
		}
		if j > 0 && toks[j].text == "=" {
			j = nextSig(toks, j)
		}
		if j < 0 {
			continue
		}
		if inExpr || t.is("USING") {
			// Inside CAST/CONVERT the charset is a bare name.
			if toks[j].kind != tokWord && toks[j].kind != tokQuotedIdent {
				continue
			}
		} else if !isNameToken(toks[j], scope) {
			continue
		}
		name := toks[j].name()
		if toks[j].kind == tokString && len(name) >= 2 {
			name = name[1 : len(name)-1]
		}
		if isCollate {
			resolve := p.collations.ResolveDetail
			if t.is("COLLATE") && strings.EqualFold(explicitCharsetBefore(toks, i), compat.CollationCharset(name)) {
				// The expression keeps its own charset: so must the collation.
				resolve = p.collations.ResolveInCharset
			}
			if dst, changed, fallback := resolve(name); changed {
				if c := compat.Caveats(name, dst, fallback); len(c) > 0 {
					p.report.Warnf("collation %s mapped to %s: %s", strings.ToLower(name), dst, strings.Join(c, "; "))
				}
				toks[j] = token{kind: tokWord, text: dst}
			}
		} else if dst, ok := p.charsets[strings.ToLower(name)]; ok {
			text := dst.charset
			// No COLLATE inside CAST/CONVERT: it is not valid there.
			if dst.collation != "" && !inExpr && !t.is("USING") && !hasCollateNearby(toks, i, j) {
				text += " COLLATE " + dst.collation
			}
			toks[j] = token{kind: tokWord, text: text}
		} else if !compat.CharsetSupported(name) {
			p.report.Warnf("charset %s is not supported by TiDB and has no charset_mappings entry", name)
		}
		i = j
	}
	return toks
}

// ctasStart returns the index where the query of CREATE TABLE ... [AS]
// SELECT starts (a top-level SELECT or WITH, or AS before one), or len(toks).
func ctasStart(toks []token) int {
	depth := 0
	for i, t := range toks {
		switch t.text {
		case "(":
			depth++
		case ")":
			depth--
		}
		if depth != 0 || !t.significant() {
			continue
		}
		if t.is("SELECT") || t.is("WITH") {
			return i
		}
		if t.is("AS") {
			if n := nextSig(toks, i); n > 0 && (toks[n].is("SELECT") || toks[n].is("WITH") || toks[n].text == "(") {
				return i
			}
		}
	}
	return len(toks)
}

// parenDepth returns how many parentheses are open at i.
func parenDepth(toks []token, i int) int {
	depth := 0
	for j := 0; j < i; j++ {
		switch toks[j].text {
		case "(":
			depth++
		case ")":
			depth--
		}
	}
	return depth
}

func explicitCharsetBefore(toks []token, i int) string {
	p := prevSig(toks, i)
	if p < 0 {
		return ""
	}
	if toks[p].kind == tokString {
		if q := prevSig(toks, p); q >= 0 && toks[q].kind == tokWord && strings.HasPrefix(toks[q].text, "_") {
			return strings.ToLower(toks[q].text[1:])
		}
		return ""
	}
	if toks[p].text != ")" {
		return ""
	}
	depth := 0
	for j := p; j >= 0; j-- {
		switch toks[j].text {
		case ")":
			depth++
		case "(":
			depth--
			if depth != 0 {
				continue
			}
			if fn := prevSig(toks, j); fn < 0 || !toks[fn].is("CAST") && !toks[fn].is("CONVERT") {
				return ""
			}
			// The charset named inside the call: last CHARSET/SET/USING name.
			cs := ""
			for k := j + 1; k < p; k++ {
				if toks[k].is("CHARSET") || toks[k].is("SET") || toks[k].is("USING") {
					if n := nextSig(toks, k); n > 0 && n < p {
						cs = strings.ToLower(strings.Fields(toks[n].name())[0])
					}
				}
			}
			return cs
		}
	}
	return ""
}

// mapIntroducer maps a charset introducer (_latin1'abc') like a column of
// that charset, so comparisons with converted columns stay valid (TiDB, like
// MySQL 8, rejects latin1 EXPLICIT vs utf8mb4: error 1267). An introducer
// says how to read the literal's bytes, so only ASCII literals, whose bytes
// are the same in both charsets, are rewritten; others are reported.
func (p *preprocessor) mapIntroducer(toks []token, i int) {
	cs := strings.ToLower(toks[i].text[1:])
	dst, ok := p.charsets[cs]
	j := nextSig(toks, i)
	if !ok || j < 0 || toks[j].kind != tokString || toks[j].text[0] == '`' {
		return
	}
	for k := 0; k < len(toks[j].text); k++ {
		if toks[j].text[k] >= 0x80 {
			p.report.Warnf("literal %s%s keeps charset %s (non-ASCII bytes cannot be re-labelled safely); comparisons with converted columns may fail (error 1267)", toks[i].text, toks[j].text, cs)
			return
		}
	}
	toks[i] = token{kind: tokWord, text: "_" + dst.charset}
}

// insideCall reports whether toks[i] is within the parentheses of a call to
// one of the named functions.
func insideCall(toks []token, i int, names ...string) bool {
	depth := 0
	for j := i - 1; j >= 0; j-- {
		switch toks[j].text {
		case ")":
			depth++
		case "(":
			if depth > 0 {
				depth--
				continue
			}
			fn := prevSig(toks, j)
			for _, n := range names {
				if fn >= 0 && toks[fn].is(n) {
					return true
				}
			}
		}
	}
	return false
}

// isNameToken reports a token that can be a charset or collation name. In
// DML a string literal is data, never a name.
func isNameToken(t token, scope mapScope) bool {
	switch t.kind {
	case tokWord, tokQuotedIdent:
		return true
	case tokString:
		return scope != mapDML
	}
	return false
}

// isColumnName reports a CHARSET/CHARACTER word at i that starts a column
// definition ("(charset varchar(10)") rather than a table option
// ("ENGINE=InnoDB, CHARSET=latin1"): it follows '(' or ',' and is not
// followed by '=' or a charset name.
func (p *preprocessor) isColumnName(toks []token, i int) bool {
	if toks[i].is("COLLATE") {
		return false
	}
	pv := prevSig(toks, i)
	afterOldName := false // CHANGE [COLUMN] old <new name>
	if pv >= 0 && (toks[pv].kind == tokWord || toks[pv].kind == tokQuotedIdent) {
		if pp := prevSig(toks, pv); pp >= 0 && (toks[pp].is("CHANGE") || toks[pp].is("COLUMN") && prevSig(toks, pp) >= 0 && toks[prevSig(toks, pp)].is("CHANGE")) {
			afterOldName = true
		}
	}
	if pv < 0 || !afterOldName && toks[pv].text != "," && toks[pv].text != "(" &&
		!toks[pv].is("COLUMN") && !toks[pv].is("ADD") && !toks[pv].is("MODIFY") && !toks[pv].is("CHANGE") {
		return false
	}
	n := nextSig(toks, i)
	if n < 0 || toks[n].text == "=" || toks[n].is("SET") {
		return false
	}
	_, mapped := p.charsets[strings.ToLower(toks[n].name())]
	return !mapped && !compat.CharsetSupported(toks[n].name())
}

// hasCollateNearby reports a COLLATE belonging to the same charset clause:
// right after the charset name, or right before the (DEFAULT) CHARSET
// keyword at kw ("COLLATE=x DEFAULT CHARSET=y").
func hasCollateNearby(toks []token, kw, name int) bool {
	if n := nextSig(toks, name); n > 0 && toks[n].is("COLLATE") {
		return true
	}
	q := prevSig(toks, kw)
	if q >= 0 && toks[q].is("DEFAULT") {
		q = prevSig(toks, q)
	}
	if q < 0 || toks[q].kind == tokPunct {
		return false
	}
	q = prevSig(toks, q) // the collation name; now its keyword or '='
	if q >= 0 && toks[q].text == "=" {
		q = prevSig(toks, q)
	}
	return q >= 0 && toks[q].is("COLLATE")
}

// isCollationVariable reports collation_connection / _database / _server,
// set by mariadb-dump around stored programs (e.g. "SET collation_connection
// = latin1_swedish_ci"). @@ and @@session. prefixes are separate tokens or
// part of the word.
func isCollationVariable(t token) bool {
	if t.kind != tokWord && t.kind != tokQuotedIdent {
		return false
	}
	switch strings.ToLower(strings.TrimLeft(t.name(), "@")) {
	case "collation_connection", "collation_database", "collation_server":
		return true
	}
	return false
}

// tableName returns the name following TABLE [IF NOT EXISTS] and the index
// of the last token of that name.
func tableName(toks []token) (string, int) {
	i := 0
	for ; i < len(toks); i++ {
		if toks[i].is("TABLE") {
			break
		}
	}
	j := nextSig(toks, i)
	if j > 0 && toks[j].is("IF") {
		j = nextSig(toks, nextSig(toks, nextSig(toks, j)))
	}
	if j < 0 {
		return "", -1
	}
	name := toks[j].name()
	if k := nextSig(toks, j); k > 0 && toks[k].text == "." {
		if m := nextSig(toks, k); m > 0 {
			return toks[m].name(), m
		}
	}
	return name, j
}

// rewriteCreateTable fixes column types, column attributes, MariaDB-only
// index kinds and table options of a CREATE TABLE.
func (p *preprocessor) rewriteCreateTable(toks []token) []token {
	table, end := tableName(toks)
	if end < 0 {
		return toks
	}
	open := nextSig(toks, end)
	if open < 0 || toks[open].text != "(" {
		return toks // CREATE TABLE ... LIKE / AS SELECT
	}
	bodyEnd := matchParen(toks, open)
	if bodyEnd < 0 {
		return toks
	}

	items := splitItems(toks, open+1, bodyEnd)
	var drop [][2]int
	for _, it := range items {
		first := firstSig(toks, it[0], it[1])
		if first < 0 {
			continue
		}
		w := strings.ToUpper(toks[first].text)
		if w == "LIKE" {
			return toks // CREATE TABLE t (LIKE other)
		}
		if w == "PERIOD" {
			if n := nextSig(toks, first); n < 0 || !toks[n].is("FOR") {
				w = "" // a column named period
			}
		}
		if toks[first].kind == tokWord && keyItemWords[w] {
			switch w {
			case "SPATIAL":
				p.report.Warnf("table %s: dropped SPATIAL index (TiDB has no spatial types; geometry columns become LONGBLOB)", table)
				drop = append(drop, it)
			case "FULLTEXT":
				p.report.Warnf("table %s: FULLTEXT index kept but not created by TiDB (self-hosted TiDB ignores it); MATCH ... AGAINST queries will fail", table)
			case "PERIOD":
				p.report.Warnf("table %s: dropped PERIOD FOR definition (TiDB has no temporal tables)", table)
				drop = append(drop, it)
			}
			continue
		}
		p.rewriteColumnDef(toks, first, it[1], table)
	}

	p.rewriteTableOptions(toks, bodyEnd+1, len(toks), table)
	return removeItems(toks, drop)
}

// rewriteAlterTable applies the column and option rewrites to the
// ADD/MODIFY/CHANGE column specs of an ALTER TABLE.
func (p *preprocessor) rewriteAlterTable(toks []token) []token {
	table, end := tableName(toks)
	if end < 0 {
		return toks
	}
	for _, it := range splitItems(toks, end+1, len(toks)) {
		i := firstSig(toks, it[0], it[1])
		if i < 0 {
			continue
		}
		var col int
		switch {
		case toks[i].is("ADD"):
			col = nextSig(toks, i)
			if col > 0 && toks[col].is("COLUMN") {
				col = nextSig(toks, col)
			}
			col = skipIfExists(toks, col)
			if col > 0 && toks[col].text == "(" {
				if c := matchParen(toks, col); c > 0 {
					for _, sub := range splitItems(toks, col+1, c) {
						if f := firstSig(toks, sub[0], sub[1]); f >= 0 {
							p.rewriteColumnDef(toks, f, sub[1], table)
						}
					}
				}
				continue
			}
			if col < 0 || toks[col].kind == tokWord && keyItemWords[strings.ToUpper(toks[col].text)] || toks[col].is("PARTITION") {
				continue
			}
		case toks[i].is("MODIFY"):
			col = nextSig(toks, i)
			if col > 0 && toks[col].is("COLUMN") {
				col = nextSig(toks, col)
			}
			col = skipIfExists(toks, col)
		case toks[i].is("CHANGE"):
			col = nextSig(toks, i)
			if col > 0 && toks[col].is("COLUMN") {
				col = nextSig(toks, col)
			}
			col = nextSig(toks, skipIfExists(toks, col)) // the new name
		case isTableOption(toks, i):
			p.rewriteTableOptions(toks, it[0], it[1], table)
			continue
		default:
			continue // RENAME, ALTER COLUMN, DROP, ...: names, not options
		}
		if col > 0 && col < it[1] {
			p.rewriteColumnDef(toks, col, it[1], table)
		}
	}
	return toks
}

// skipIfExists skips "IF [NOT] EXISTS" at i (MariaDB's ALTER TABLE ADD
// COLUMN IF NOT EXISTS, MODIFY/CHANGE IF EXISTS) and returns the index of the
// following token.
func skipIfExists(toks []token, i int) int {
	if i < 0 || !toks[i].is("IF") {
		return i
	}
	j := nextSig(toks, i)
	if j > 0 && toks[j].is("NOT") {
		j = nextSig(toks, j)
	}
	if j > 0 && toks[j].is("EXISTS") {
		return nextSig(toks, j)
	}
	return i
}

// isTableOption reports an ALTER TABLE spec that is a table option to
// drop (NAME [=] value, or WITH SYSTEM VERSIONING).
func isTableOption(toks []token, i int) bool {
	t := toks[i]
	return (t.kind == tokWord || t.kind == tokQuotedIdent) && droppedTableOptions[strings.ToUpper(t.name())] || t.is("WITH")
}

// rewriteColumnDef rewrites one column definition spanning toks[name:end]:
// unsupported types, and MariaDB-only attributes outside parentheses.
func (p *preprocessor) rewriteColumnDef(toks []token, name, end int, table string) {
	col := toks[name].name()
	typ := nextSig(toks, name)
	if typ < 0 || typ >= end || toks[typ].kind != tokWord {
		return
	}
	src := strings.ToLower(toks[typ].text)
	repl, convert := typeReplacements[src]
	if after := nextSig(toks, typ); convert && after > 0 && after < end && toks[after].text == "(" {
		convert = false // a function call, not a type
	}
	if convert {
		toks[typ] = token{kind: tokWord, text: repl}
		if repl == "longblob" {
			p.report.Warnf("table %s: column %s %s became LONGBLOB (TiDB has no spatial types; WKB bytes are kept, spatial functions are not)", table, col, strings.ToUpper(src))
		}
	}

	depth := 0
	hasDefault, notNullEnd := false, -1
	for i := typ + 1; i < end; i++ {
		t := toks[i]
		switch t.text {
		case "(":
			depth++
			continue
		case ")":
			depth--
			continue
		}
		if depth != 0 || t.kind != tokWord {
			continue
		}
		switch {
		case t.is("DEFAULT"):
			hasDefault = true
		case t.is("NULL"):
			if j := prevSig(toks, i); j > typ && toks[j].is("NOT") {
				notNullEnd = i
			}
		case t.is("PERSISTENT"): // MariaDB's synonym for STORED
			toks[i] = token{kind: tokWord, text: "STORED"}
		case t.is("INVISIBLE"):
			p.report.Warnf("table %s: column %s is no longer INVISIBLE (TiDB has no invisible columns; SELECT * now returns it)", table, col)
			blank(toks, i, i+1)
		case t.is("COMPRESSED"):
			stop := i + 1
			if j := nextSig(toks, i); j > 0 && j < end && toks[j].text == "=" {
				stop = nextSig(toks, j) + 1
			}
			p.report.Warnf("table %s: dropped column compression on %s", table, col)
			blank(toks, i, stop)
		case t.is("WITH") || t.is("WITHOUT"):
			if j := nextSig(toks, i); j > 0 && toks[j].is("SYSTEM") {
				if k := nextSig(toks, j); k > 0 && toks[k].is("VERSIONING") {
					blank(toks, i, k+1)
				}
			}
		case t.is("GENERATED"):
			// GENERATED ALWAYS AS ROW START|END (system-versioning columns).
			j1 := nextSig(toks, i)
			j2 := nextSig(toks, j1)
			j3 := nextSig(toks, j2)
			j4 := nextSig(toks, j3)
			if j1 > 0 && toks[j1].is("ALWAYS") && j2 > 0 && toks[j2].is("AS") && j3 > 0 && toks[j3].is("ROW") && j4 > 0 && (toks[j4].is("START") || toks[j4].is("END")) {
				p.report.Warnf("table %s: column %s is a plain column now (was GENERATED ALWAYS AS ROW %s)", table, col, strings.ToUpper(toks[j4].text))
				blank(toks, i, j4+1)
			}
		}
	}
	if convert && src == "uuid" && notNullEnd > 0 && !hasDefault {
		toks[notNullEnd].text += " DEFAULT ''"
	}
}

// rewriteTableOptions drops MariaDB-only table options and WITH SYSTEM
// VERSIONING in toks[from:to] (outside parentheses).
func (p *preprocessor) rewriteTableOptions(toks []token, from, to int, table string) {
	depth := 0
	for i := from; i < to; i++ {
		t := toks[i]
		switch t.text {
		case "(":
			depth++
			continue
		case ")":
			depth--
			continue
		}
		if depth != 0 || !t.significant() {
			continue
		}
		if t.is("AS") || t.is("SELECT") || t.is("IGNORE") || t.is("REPLACE") || t.is("WITH") && !isSystemVersioning(toks, i) {
			return // CREATE TABLE ... AS SELECT: the rest is a query
		}
		name := strings.ToUpper(t.name())
		if (t.kind == tokWord || t.kind == tokQuotedIdent) && droppedTableOptions[name] {
			j := nextSig(toks, i)
			if j > 0 && toks[j].text == "=" {
				j = nextSig(toks, j)
			}
			if j > 0 && j < to {
				blank(toks, i, j+1)
				dropSeparator(toks, i, j+1)
				i = j
			}
			continue
		}
		if t.is("WITH") {
			if j := nextSig(toks, i); j > 0 && toks[j].is("SYSTEM") {
				if k := nextSig(toks, j); k > 0 && toks[k].is("VERSIONING") {
					p.report.Warnf("table %s: dropped WITH SYSTEM VERSIONING (TiDB has no system-versioned tables; history is not migrated)", table)
					blank(toks, i, k+1)
					i = k
				}
			}
		}
	}
}

// rewriteSetval converts MariaDB's SETVAL(seq, value, is_used[, round]).
// When seq was created earlier in the same input the next value is folded
// into its START WITH, which is exact for ascending and descending
// sequences (TiDB ignores SETVAL on a fresh descending sequence and would
// hand out issued values again). An exhausted NOCYCLE sequence starts at
// its last value and gets a DO NEXTVAL, returned as exhaust, so it stays
// exhausted. Without a preceding CREATE SEQUENCE the call becomes TiDB's
// two-argument SETVAL, with a warning to check the position. It returns
// false to drop the statement.
func (p *preprocessor) rewriteSetval(toks []token, seqs map[string]*seqState, db string) (_ []token, keep bool, exhaust *seqState) {
	open := -1
	for i, t := range toks {
		if t.is("SETVAL") {
			open = nextSig(toks, i)
			break
		}
	}
	argsEnd := matchParen(toks, open)
	if open < 0 || argsEnd < 0 {
		return toks, true, nil
	}
	args := splitItems(toks, open+1, argsEnd)
	if len(args) < 3 {
		return toks, true, nil
	}
	num := func(r [2]int) (int64, error) {
		return strconv.ParseInt(strings.Join(strings.Fields(argText(toks, r)), ""), 10, 64)
	}
	name := strings.TrimSpace(argText(toks, args[0]))
	value, err1 := num(args[1])
	used, err2 := num(args[2])
	if err1 != nil || err2 != nil {
		return toks, true, nil
	}
	if len(args) == 4 {
		if round, err := num(args[3]); err != nil || round != 0 {
			p.report.Warnf("sequence %s: SETVAL round argument dropped (TiDB does not track cycle rounds)", name)
		}
	}
	// Keyed by database and name: `db`.`seq`, or seq in the USE database.
	key := db + "." + strings.ToLower(name)
	if last := prevSig(toks, args[0][1]); last >= args[0][0] {
		schema := db
		if dot := prevSig(toks, last); dot >= args[0][0] && toks[dot].text == "." {
			if s := prevSig(toks, dot); s >= args[0][0] {
				schema = strings.ToLower(toks[s].name())
			}
		}
		key = schema + "." + strings.ToLower(toks[last].name())
	}
	if st, ok := seqs[key]; ok {
		next := value
		if used != 0 {
			next = value + st.inc
		}
		switch {
		case next >= st.min && next <= st.max:
			st.setStart(next)
		case st.cycle:
			if st.inc > 0 {
				st.setStart(st.min)
			} else {
				st.setStart(st.max)
			}
			p.report.Warnf("sequence %s: wrapped around (CYCLE); it restarts at its bound", name)
		default:
			last := st.max
			if st.inc < 0 {
				last = st.min
			}
			st.setStart(last)
			exhaust = st
			p.report.Warnf("sequence %s is exhausted in the source; it stays exhausted", name)
		}
		return nil, false, exhaust
	}
	p.report.Warnf("sequence %s: SETVAL without a preceding CREATE SEQUENCE became a two-argument SETVAL; check its position after the import (TiDB ignores it on a fresh descending sequence)", name)
	repl := []token{{kind: tokWord, text: "DO SETVAL(" + name + ", " + strconv.FormatInt(value, 10) + ")"}}
	return repl, true, nil
}

func argText(toks []token, r [2]int) string {
	return joinTokens(toks[r[0]:r[1]])
}

func parseSequence(toks []token) (string, *seqState) {
	st := &seqState{toks: toks, startIdx: -1, startNeg: -1, nameIdx: -1, inc: 1, min: -1 << 63, max: 1<<63 - 1}
	minSet, maxSet := false, false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if !t.significant() {
			continue
		}
		switch {
		case t.is("SEQUENCE") && st.nameIdx < 0:
			j := nextSig(toks, i)
			if j > 0 && toks[j].is("IF") {
				j = nextSig(toks, nextSig(toks, nextSig(toks, j)))
			}
			if j > 0 {
				if k := nextSig(toks, j); k > 0 && toks[k].text == "." {
					st.schema = strings.ToLower(toks[j].name())
					j = nextSig(toks, k)
				}
			}
			if j > 0 {
				st.nameIdx = j
				st.name = toks[j].text
			}
		case t.is("START"):
			if j, _, ok := seqNumber(toks, i, "WITH"); ok {
				if toks[j].text == "-" {
					st.startNeg, st.startIdx = j, nextSig(toks, j)
				} else {
					st.startIdx = j
				}
			}
		case t.is("INCREMENT"):
			if _, v, ok := seqNumber(toks, i, "BY"); ok {
				st.inc = v
			}
		case t.is("MINVALUE"):
			if _, v, ok := seqNumber(toks, i, ""); ok {
				st.min, minSet = v, true
			}
		case t.is("MAXVALUE"):
			if _, v, ok := seqNumber(toks, i, ""); ok {
				st.max, maxSet = v, true
			}
		case t.is("CYCLE"):
			st.cycle = true
		}
	}
	// Defaults as in MariaDB/TiDB: ascending 1..max, descending min..-1.
	if !minSet && st.inc > 0 {
		st.min = 1
	}
	if !maxSet && st.inc < 0 {
		st.max = -1
	}
	if st.nameIdx < 0 {
		return "", st
	}
	return toks[st.nameIdx].name(), st
}

// seqNumber parses "KEYWORD [joiner|=] [-]N" starting at the keyword i and
// returns the index of the number's first token and its value.
func seqNumber(toks []token, i int, joiner string) (int, int64, bool) {
	j := nextSig(toks, i)
	if j > 0 && (toks[j].text == "=" || joiner != "" && toks[j].is(joiner)) {
		j = nextSig(toks, j)
	}
	if j < 0 {
		return -1, 0, false
	}
	text := toks[j].text
	if text == "-" {
		k := nextSig(toks, j)
		if k < 0 {
			return -1, 0, false
		}
		text = "-" + toks[k].text
	}
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return -1, 0, false
	}
	return j, v, true
}

// matchParen returns the index of the ')' matching the '(' at open, or -1.
func matchParen(toks []token, open int) int {
	if open < 0 || open >= len(toks) || toks[open].text != "(" {
		return -1
	}
	depth := 0
	for i := open; i < len(toks); i++ {
		switch toks[i].text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitItems splits toks[from:to] at top-level commas into [start, end)
// ranges (commas excluded).
func splitItems(toks []token, from, to int) [][2]int {
	var items [][2]int
	depth, start := 0, from
	for i := from; i < to; i++ {
		switch toks[i].text {
		case "(":
			depth++
		case ")":
			depth--
		case ",":
			if depth == 0 {
				items = append(items, [2]int{start, i})
				start = i + 1
			}
		}
	}
	return append(items, [2]int{start, to})
}

func firstSig(toks []token, from, to int) int {
	for i := from; i < to; i++ {
		if toks[i].significant() {
			return i
		}
	}
	return -1
}

func prevSig(toks []token, i int) int {
	for j := i - 1; j >= 0; j-- {
		if toks[j].significant() {
			return j
		}
	}
	return -1
}

// dropSeparator removes the comma that separated a just-blanked option or
// spec toks[from:to] from its neighbour (table options and ALTER specs may
// be comma-separated), so no dangling comma is left.
func dropSeparator(toks []token, from, to int) {
	if p := prevSig(toks, from); p >= 0 && toks[p].text == "," {
		blank(toks, p, p+1)
	} else if n := nextSig(toks, to-1); n >= 0 && toks[n].text == "," {
		blank(toks, n, n+1)
	}
}

// isSystemVersioning reports "WITH SYSTEM VERSIONING" at i.
func isSystemVersioning(toks []token, i int) bool {
	j := nextSig(toks, i)
	k := nextSig(toks, j)
	return j > 0 && toks[j].is("SYSTEM") && k > 0 && toks[k].is("VERSIONING")
}

// blank empties toks[from:to] in place (keeping indexes stable).
func blank(toks []token, from, to int) {
	for i := from; i < to && i < len(toks); i++ {
		toks[i] = token{kind: tokSpace, text: ""}
	}
}

// removeItems blanks the given body items together with one adjacent comma.
func removeItems(toks []token, items [][2]int) []token {
	for _, it := range items {
		if c := prevSig(toks, it[0]); c >= 0 && toks[c].text == "," {
			blank(toks, c, it[1])
		} else if it[1] < len(toks) && toks[it[1]].text == "," {
			blank(toks, it[0], it[1]+1)
		} else {
			blank(toks, it[0], it[1])
		}
	}
	return toks
}
