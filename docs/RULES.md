# Transformation Rules

## Overview

The mariadb2tidb tool uses a rule-based system to transform MariaDB SQL schemas into a TiDB-compatible format. Each rule addresses a specific compatibility difference, with the relevant TiDB documentation linked.

## Rule System

### Rule Interface
All rules implement the `Rule` interface:
```go
type Rule interface {
    Name() string                          // Unique rule identifier
    Description() string                   // Human-readable description
    Priority() int                         // Execution order (lower = earlier)
    ShouldApply(node ast.Node) bool        // Determine if rule applies to AST node
    Apply(node ast.Node) (ast.Node, error) // Transform the AST node
}
```

### Registry
- Rules run in priority order (lower number first)
- `enabled_rules` / `disabled_rules` in the YAML config control which rules are active; an empty `enabled_rules` list means "all rules"; unknown rule names are an error

## Active Rules

### Collation (Priority: 100)
Normalizes charsets and collations using the configurable `charset_mappings` / `collation_mappings`:
- `utf8mb4_unicode_*` → `utf8mb4_0900_ai_ci` (covers `utf8mb4_unicode_520_ci`, which TiDB does not support)
- `latin1_swedish_ci` → `utf8mb4_0900_ai_ci`
- `DEFAULT CHARSET=latin1` → `DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`

TiDB supports `utf8mb4_0900_ai_ci` as a client/default collation from **v7.4.0**; target older versions by overriding the mappings (e.g. to `utf8mb4_general_ci`).
Docs: <https://docs.pingcap.com/tidb/stable/character-set-and-collation>

Collations TiDB does not support at all (for example MariaDB 11's default `utf8mb4_uca1400_ai_ci`) are resolved already in the loader, because the parser rejects them: configured exact names first, then the most specific configured pattern, then a fallback that keeps case sensitivity: it is decided first (`*_bin`/`*_cs` stay case-sensitive, so a unique key over `'a'` and `'A'` still loads), then a collation of the charset, or of its mapped charset, is picked (`*_ci` → the mapping's target collation, `utf8mb4_0900_ai_ci` or whatever `utf8mb4_unicode_ci` is mapped to; `*_bin`/`*_cs` → `utf8mb4_bin`; `utf8mb3_*` → `utf8mb3_general_ci`/`utf8mb3_bin`; `gbk_*` → `gbk_chinese_ci`/`gbk_bin`). A collation TiDB supports is still remapped when its charset is (`latin1_bin` under `latin1 → utf8mb4` becomes `utf8mb4_bin`; TiDB rejects the mixed pair). Charsets without a case-insensitive collation in TiDB (`ascii`, unmapped `latin1`) end up case-sensitive, which is reported.

Every collation change that alters comparisons is reported, once per pair: a fallback without an exact equivalent; lost language rules (`latin1_swedish_ci`, `utf8mb4_german2_ci`, … → a generic collation); lost accent sensitivity; gained case sensitivity; and a padding change. TiDB's `utf8mb4_0900_*` collations are NO PAD while MariaDB's (except `*_nopad_*`) are PAD SPACE, so with the default mapping `'a'` and `'a '` stop being equal; MariaDB `*_nopad_bin` maps to the NO PAD `utf8mb4_0900_bin`. Map to `utf8mb4_unicode_ci` or `utf8mb4_general_ci` (PAD SPACE) in `collation_mappings` where trailing-space equality matters. Mapping an accent-sensitive `*_as_ci` collation to an accent-insensitive one is reported: values such as `e` and `é` become equal and unique keys may reject rows.

### Engine (Priority: 150)
Replaces storage engines TiDB rejects with error 1286 (MariaDB's `Aria`, `ColumnStore`, `Spider`, `CONNECT`, `S3`, `ROCKSDB`, `MRG_MyISAM`, `TokuDB`, `Mroonga`, …) with `InnoDB`. Engines TiDB accepts and ignores (`InnoDB`, `MyISAM`, `MEMORY`/`HEAP`, `ARCHIVE`, `CSV`, `BLACKHOLE`, `FEDERATED`, `MERGE`, `NDB`, `EXAMPLE`) are kept. `UNION=(…)` and `INSERT_METHOD` of MERGE tables, which TiDB rejects (errors 8232/8233), are removed: the table becomes an ordinary empty table, which is reported.

### KeyLength (Priority: 300)
Caps **index key prefix lengths** at 768 characters (3072 bytes in utf8mb4, TiDB's default `max-index-length`). Applies to explicit oversized prefixes and to indexes over `CHAR`/`VARCHAR` columns wider than 768 characters. Column definitions are never truncated — TiDB supports `VARCHAR` up to 16383 characters.
Docs: <https://docs.pingcap.com/tidb/stable/tidb-limitations>

### IndexPrefix (Priority: 350)
Adds a 255-character prefix to indexed `TEXT`/`BLOB` columns that lack one; TiDB (like MySQL) cannot index these types without an explicit prefix.

### TextBlobDefaults (Priority: 400)
Removes `DEFAULT` values from `TEXT`/`BLOB`/`JSON` columns. TiDB rejects literal defaults on these types (expression defaults exist from v8.0.0, but stripping keeps output loadable on all supported versions).
Docs: <https://docs.pingcap.com/tidb/stable/data-type-default-values>

### ZeroDate (Priority: 450)
Zero-date defaults (`DEFAULT '0000-00-00 00:00:00'`, any zero month or day, in every form MySQL reads: `0`, `'00000000000000'`, `'0000/00/00'`, `20200010`) load on MariaDB but fail on TiDB with error 1067 under its default sql_mode (`NO_ZERO_DATE`, `NO_ZERO_IN_DATE`). They do load where the session sql_mode is permissive: `mariadb-dump` output sets one, and TiDB Lightning uses one. `zero_date_defaults: keep` (default) leaves them and warns; `drop` removes the `DEFAULT` clause.

### JsonCheck (Priority: 500)
Removes `CHECK (JSON_VALID(...))` constraints that MariaDB emits for JSON columns (MariaDB stores JSON as `LONGTEXT` + check constraint).

### FunctionDefault (Priority: 600)
Removes function-based `DEFAULT` values not in the allowlist (`allowed_default_functions` in the config; defaults to the `CURRENT_TIMESTAMP` family: `current_timestamp`, `current_date`, `current_time`, `now`, `localtime`, `localtimestamp`). Extend the allowlist when targeting TiDB ≥ v8.0.0, which supports more default expressions.
Docs: <https://docs.pingcap.com/tidb/stable/data-type-default-values>

TiDB ≥ v8.0 accepts `DEFAULT uuid()` (and `DEFAULT (UUID())`); add `uuid` to the allowlist when targeting it.

### JsonValue (Priority: 650)
TiDB has no `JSON_VALUE` (error 1305; in generated columns 3102). Every call, including nested ones, in generated columns, views, CHECK constraints and defaults, is rewritten to

```sql
CASE WHEN JSON_VALID(doc) AND JSON_TYPE(JSON_EXTRACT(doc, path)) NOT IN ('OBJECT', 'ARRAY', 'NULL')
     THEN JSON_UNQUOTE(JSON_EXTRACT(doc, path)) END
```

which returns `NULL` where MariaDB does: invalid documents (a bare `JSON_EXTRACT` would fail the whole `INSERT` with error 3140), JSON `null`, objects and arrays. Scalars may still be formatted differently (`1.50` → `1.5`, `true` instead of `1`), which is reported.

### JsonGenerated (Priority: 700) — disabled by default
Converts generated columns that use JSON functions into regular columns. Modern TiDB (v6.3+) supports JSON functions in generated columns, so this rule is only needed for legacy targets — enable it via `enabled_rules` or remove `JsonGenerated` from `disabled_rules`.
Docs: <https://docs.pingcap.com/tidb/stable/generated-columns>

### AutoRandom (Priority: 800) — opt-in
For the tables in `auto_random.tables` (or `"*"`), converts a single-column `BIGINT AUTO_INCREMENT` primary key to `AUTO_RANDOM(shard_bits, range_bits)` and marks the key `CLUSTERED`, so inserts spread over regions instead of one hot region. Defaults are `shard_bits: 5`, `range_bits: 54`: every ID stays below 2^53, exact in JavaScript and below PHP's `PHP_INT_MAX`. `range_bits: 64` (TiDB's default) allows IDs up to 2^63−1 and is reported. Not converted, with a warning: `UNSIGNED` columns (no sign bit, so IDs would exceed 2^63−1 = `PHP_INT_MAX`), non-`BIGINT` columns, composite or `NONCLUSTERED` keys. IDs are no longer in insertion order, and loading existing rows with SQL needs `SET @@allow_auto_random_explicit_insert=1` (TiDB Lightning handles it).
Docs: <https://docs.pingcap.com/tidb/stable/auto-random>

### ShardRowID (Priority: 810) — opt-in
With `shard_row_id_bits: N` (and optionally `pre_split_regions`), adds `SHARD_ROW_ID_BITS = N` to tables keyed by the hidden `_tidb_rowid`: tables without a primary key or with a `NONCLUSTERED` one. All other primary keys are clustered by default (`tidb_enable_clustered_index=ON`) and TiDB rejects the option on them (error 8200).
Docs: <https://docs.pingcap.com/tidb/stable/shard-row-id-bits>

### GeneratedValues (Priority: 900)
`mariadb-dump` writes values for generated columns in `INSERT`s (MariaDB ignores them on load); TiDB rejects such statements with error 3105. Values in generated-column positions of `INSERT`/`REPLACE` are replaced with `DEFAULT`, and `ON DUPLICATE KEY UPDATE` assignments to generated columns are dropped, using the generated columns of `CREATE TABLE` statements earlier in the same file (a full dump), keyed by database and following `USE`. `INSERT … SELECT` into generated columns cannot be fixed this way and is reported. Dumpling leaves generated columns out of its data files (it skips columns whose `SHOW COLUMNS` `Extra` is `VIRTUAL GENERATED`/`STORED GENERATED`, which MariaDB reports as well).

## Loader Preprocessing (not rules)

Some MariaDB-only syntax cannot be represented in the TiDB parser AST, so the loader fixes it before parsing. It works on tokens, not text: string literals, comments and quoted identifiers are never rewritten, and only statements it understands are touched.
- The script is split honoring the mysql client's `DELIMITER` directive (first word only, as the client reads it) and `SET sql_mode`: `mariadb-dump` writes routines created under `NO_BACKSLASH_ESCAPES` in that mode, where `'C:\'` is a complete string. Statements written in that mode get their backslashes normalized to the default escaping, and the flag is removed from `sql_mode` values, because the output always escapes backslashes. `sql_mode` values are evaluated like the server does (literals keep their case, `REPLACE` is case-sensitive, the stored mode is upper case): literals, `@@sql_mode`, user variables, `CONCAT`, `CONCAT_WS`, `REPLACE`, `UPPER`, `LOWER`, `IF` with a constant condition; `SET` inside `/*M! */` counts, `/*!999999 */` (never executed by any server) does not. Anything else keeps the current mode with a warning to check backslashes. Six-digit executable-comment versions become a bare `/*!`, which the TiDB parser can read. A UTF-8 BOM is skipped, and `*/` inside a string does not end an executable comment. Stored programs (`TRIGGER`, `PROCEDURE`, `FUNCTION`, `EVENT`, `PACKAGE`), which TiDB does not support, are dropped with a warning, along with their `DROP`/`ALTER`; MariaDB-only `/*M! … */` comments (e.g. the "enable the sandbox mode" line of `mariadb-dump` 11.x) are removed
- Column types TiDB lacks get a data-compatible replacement, in `CREATE TABLE` and in `ALTER TABLE … ADD/MODIFY/CHANGE`: `UUID` → `CHAR(36)`, `INET4` → `VARCHAR(15)`, `INET6` → `VARCHAR(39)`, spatial types → `LONGBLOB` (WKB bytes are kept); `NOT NULL` uuid columns without a default get `DEFAULT ''`; function calls and identifiers named `uuid` are left alone
- MariaDB-only column attributes are removed with a warning: `INVISIBLE` (invisible *indexes* are supported and kept), `COMPRESSED`, `WITH[OUT] SYSTEM VERSIONING`, `GENERATED ALWAYS AS ROW START|END`; `PERSISTENT` becomes `STORED`; `SPATIAL` indexes and `PERIOD FOR` definitions are dropped; `FULLTEXT` indexes are kept but reported, because self-hosted TiDB accepts the syntax without creating them
- `CREATE TABLE … SELECT`, which TiDB does not implement (error 1105), is reported
- MariaDB-only table options are dropped: `PAGE_COMPRESSED`, `PAGE_COMPRESSION_LEVEL`, `TRANSACTIONAL`, `PAGE_CHECKSUM`, `IETF_QUOTES`, `ENCRYPTED`, `ENCRYPTION_KEY_ID`, and `WITH SYSTEM VERSIONING` (reported: history is not migrated)
- Collations, including `collation_connection` / `_database` / `_server` assignments in `SET`, are resolved as described under Collation; configured charset mappings are applied and get the mapping's target collation when the clause has none (otherwise the table would fall back to TiDB's case-sensitive default `utf8mb4_bin`). `SET NAMES` / `SET CHARACTER SET` are left alone: they describe the bytes of the dump itself. In data statements (`INSERT`, `UPDATE`, `SELECT`, …), view bodies and the query of `CREATE TABLE … SELECT` only names are touched, never strings: a string after `charset =` is data, as is `(charset = 'x')` in an expression. Expressions follow the columns' charset mapping, so comparing them with converted columns stays valid (TiDB, like MySQL 8, rejects latin1 EXPLICIT vs utf8mb4 with error 1267): `CAST(… AS CHAR CHARSET x)` and `CONVERT(… USING x)` get the mapped charset (without a `COLLATE`, which is not valid there), and a `COLLATE` after them is resolved normally. An introducer such as `_latin1'abc'` is re-labelled only for ASCII literals, whose bytes are the same in both charsets; other literals keep their charset and a collation of it, with a warning
- MariaDB's `SETVAL(seq, value, is_used[, round])` is folded into the `START WITH` of the preceding `CREATE SEQUENCE` of the same database (qualified, or the `USE` database; added when missing), which is exact for ascending and descending sequences (TiDB ignores `SETVAL` on a fresh descending sequence and would hand out issued values again). An exhausted `NOCYCLE` sequence starts at its last value followed by `DO NEXTVAL`, so it stays exhausted. Without a preceding `CREATE SEQUENCE` it becomes TiDB's two-argument `SETVAL`, with a warning to check the position

## Output Check

After writing, the output is parsed again and compared with the parsed input statement by statement: every table with its columns (in order), keys (including column-level `PRIMARY KEY`/`UNIQUE`/`REFERENCES`, key order and `DESC`, foreign key targets and actions) and partitioning; views; `INSERT`s with target, columns and row and value counts; `ALTER TABLE` specs; `USE`. Types, defaults, options, key prefix lengths, clustering and `CHECK` constraints are left out because rules change them on purpose. A mismatch fails the run instead of producing a schema that silently lost something.

## Rule Constraints

- Rules must be idempotent (applying twice = applying once)
- Rules should not modify nodes they don't handle
- Rules must preserve data: never shrink column types or drop columns
- Rules should be covered by unit tests **and** a golden fixture

## Adding New Rules

1. Implement the `Rule` interface in `internal/rules/`
2. Register it in `registerDefaultRules` (`registry.go`) with an appropriate priority
3. Add unit tests and a golden fixture (`test/fixtures/<name>.sql`, then `go test ./test -run TestGolden -update`)
4. Update this documentation

## Candidate Future Rules

- **Stored programs**: emit the dropped triggers/routines into a separate file for manual porting to the application
- **Invisible columns**: rewrite `SELECT *` in views over tables whose invisible columns became visible
- **MariaDB Oracle mode** (`sql_mode=ORACLE`) syntax
