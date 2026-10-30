# Changelog

All notable changes to the mariadb2tidb project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- The integration CI job runs as a matrix: MariaDB 11.4 and 11.8 (both LTS)
  against TiDB v8.5.8 and v7.5.8, the oldest LTS the default
  `utf8mb4_0900_ai_ci` collation mapping supports.

## [1.3.0] - 2026-10-23

Tested end to end against live MariaDB 11.4 and TiDB v8.5.8: a corpus of
MariaDB features is created on MariaDB, dumped with `mariadb-dump`
(routines, triggers, events, data), converted, loaded into TiDB and compared
column by column and row by row (`make integration`, CI job `integration`).

### Fixed
- **String literals and comments are never rewritten.** The `uuid` type
  replacement and the `char(36) NOT NULL` default insertion ran as regexes
  over the whole text, so `COMMENT 'a uuid value'` and data such as
  `'generate a uuid here'` came out as `char(36)`, and a string containing
  `char(36) not null` got `default ''` inserted into it. Preprocessing now
  works on tokens and only inside DDL.
- **MariaDB 11 dumps load.** Its default collation `utf8mb4_uca1400_ai_ci`
  (and any other collation TiDB lacks) failed parsing; collations now
  resolve deterministically (configured exact name, most specific pattern,
  then a same-charset fallback that keeps case sensitivity). Wildcard
  mappings used to be matched in random map order, and the loader's mapping
  missed `COLLATE=x` and wildcard patterns entirely.
- `transform-dir` copies Dumpling data chunks (`db.table.N.sql`, CSV,
  compressed) and other files (`metadata`) unchanged instead of rewriting
  rows or dropping non-`.sql` files, so the output is a complete Lightning
  source.
- Script splitting follows the mysql client: `SET sql_mode` (a routine
  dumped under `NO_BACKSLASH_ESCAPES` no longer swallows every following
  database), `*/` inside strings of executable comments, the first word
  after `DELIMITER`, a UTF-8 BOM.
- Every collation change that alters comparisons is reported: lost
  language rules, accents, case, and padding (TiDB's `utf8mb4_0900_*` are
  NO PAD, MariaDB's are PAD SPACE, so `'a'` and `'a '` stop being equal
  under the default mapping); `*_nopad_bin` maps to NO PAD
  `utf8mb4_0900_bin`. The AST Collation rule now resolves like the loader.
- MERGE tables lose `UNION`/`INSERT_METHOD` (rejected by TiDB) with a
  warning; unknown rule names in the config are an error.
- Expressions follow the columns' charset mapping: `CAST(... CHARSET x)`,
  `CONVERT(... USING x)`, ASCII `_latin1'...'` literals and a `COLLATE`
  after them (otherwise comparisons with converted columns fail with
  1267/1253); an explicit table/database `COLLATE` is no longer overridden
  by a charset mapping's collation.
- Sequences are matched by database as well as name; `CREATE TABLE ...
  SELECT` (unsupported by TiDB) is reported; accent-sensitive
  `latin1_general_ci` and `*_as_*` mappings are reported.
- Collation fallback keeps case sensitivity (`latin1_general_cs` no longer
  becomes `utf8mb4_0900_ai_ci`, which made unique keys reject `'a'`/`'A'`),
  and a supported collation is remapped with its charset (`latin1_bin` →
  `utf8mb4_bin`).
- From review: an apostrophe in a `-- ` comment inside an executable
  comment no longer opens a string (it swallowed the rest of the dump);
  `` `sql_mode` `` quoted and numeric `SET sql_mode = 1048576` are tracked,
  and the bit is cleared in the output; INSERTs after an ALTER that changes
  a table's columns are no longer rewritten from a stale layout; symlinked
  subdirectories in the output are refused; nested `JSON_VALUE` deeper than
  4 levels is an error instead of exponentially large output.
- `ALTER TABLE t ALTER COLUMN c SET DEFAULT ...` no longer panics
  (TextBlobDefaults); in v1.2.0 it did.
- A failing rule fails the run instead of being logged and skipped.
- Default log output is warnings only; errors are no longer printed twice.

### Added
- Stored programs (triggers, procedures, functions, events, packages) are
  dropped with a warning; `DELIMITER` blocks and MariaDB-only `/*M! */`
  comments are handled.
- Column types `INET4`/`INET6` (→ `VARCHAR`) and spatial types
  (→ `LONGBLOB`, WKB kept); MariaDB-only attributes (`INVISIBLE`,
  `COMPRESSED`, system versioning, `GENERATED ALWAYS AS ROW START/END`),
  `SPATIAL` indexes, `PERIOD FOR` and table options (`PAGE_COMPRESSED`,
  `TRANSACTIONAL`, `PAGE_CHECKSUM`, `IETF_QUOTES`, `WITH SYSTEM VERSIONING`)
  are removed with warnings; `FULLTEXT` indexes are reported (TiDB accepts
  them without creating them).
- Sequences keep their position: MariaDB's `SETVAL(seq, n, is_used)` is
  folded into `START WITH`, exact for descending sequences too.
- Rules: `Engine` (Aria, ColumnStore, … → InnoDB), `JsonValue`
  (`JSON_VALUE` → a guarded `JSON_UNQUOTE(JSON_EXTRACT())` with MariaDB's
  NULLs for invalid JSON, null, objects, arrays), `ZeroDate`
  (`zero_date_defaults: keep|drop`), `GeneratedValues` (values for
  generated columns in INSERTs become `DEFAULT`), and opt-in `AutoRandom`
  (`auto_random.tables`, refuses `UNSIGNED`, `range_bits: 54` keeps IDs
  below 2^53) and `ShardRowID` (`shard_row_id_bits`).
- Output check: the output is re-parsed and compared statement by
  statement (tables, columns, all keys, partitioning, INSERT rows, ALTER
  specs, USE); any loss fails the run.
- `transform-dir` refuses an output directory equal to or nested with the
  input (copying onto itself truncated files), rejects symlinks instead of
  skipping them, follows a symlinked input root, and transforms gzip schema
  files.
- Config files are decoded strictly (unknown keys are errors) and validated
  against TiDB's limits.
- Migration warnings are collected and printed at the end of a run.
- Integration tests and CI job against MariaDB 11.4 and TiDB v8.5.8.

### Changed
- Unique keys named `uuid` keep their name (the `uuid_key` rename only
  worked around the old text replacement); only uuid-converted columns get
  `DEFAULT ''`, not every `char(36) NOT NULL` column.

## [1.2.0] - 2026-10-22

### Fixed
- **Output no longer drops columns or identifier quoting.** The writer
  re-parsed TiDB's restored SQL with the vitess (MySQL) parser just to
  pretty-print it. For any `CREATE TABLE` with syntax vitess does not know
  (`AUTO_RANDOM`, `SHARD_ROW_ID_BITS`, `TTL`, MariaDB `PAGE_COMPRESSED`, …)
  vitess silently returned a partial DDL, so the output was `create table t;`
  with every column gone — and it still validated. It also stripped
  backticks from all names except vitess's own keywords, producing invalid
  DDL for TiDB reserved words such as `until`, `except`, `intersect`,
  `dual`. The writer now uses the TiDB parser's own restore only.
- `formatting` options (`indent_size`, `uppercase_keywords`,
  `backquote_identifiers`, `single_quote_strings`) are now honored; they were
  previously ignored.
- **Backslashes in string literals are escaped.** `DEFAULT 'C:\\new'` was
  written as `'C:\new'` (loads as a newline) and `DEFAULT '\\'` as `'\'`
  (invalid SQL); affects defaults, COMMENTs and INSERT values.
- NUL bytes in string literals are written as `\0` instead of a raw byte,
  which the `mysql` client rejects outside `--binary-mode`.
- `NewDirProcessor(nil)` / `NewWriterWithConfig(nil)` use the default
  configuration instead of panicking.
- Output files and stdout now end with a trailing newline.
- Preprocessor no longer panics (or inserts `default ''` at a wrong offset)
  on input with invalid UTF-8 or length-changing letters near a `char(36)`
  column; found by fuzzing.
- A TiDB parser panic (e.g. a decimal literal over 81 digits) is reported as
  a normal load/validate error instead of crashing the run.

### Changed
- Output format: uppercase keywords, every identifier backtick-quoted,
  2-space indent, table options as TiDB writes them
  (`DEFAULT CHARACTER SET = UTF8MB4`). String literals keep their charset
  introducer (`DEFAULT _UTF8MB4'x'`) so values don't depend on the import
  session's `SET NAMES`; empty strings are written as plain `''`.
- TiDB parser pinned to the v8.5.8 (LTS) release commit instead of an
  arbitrary master snapshot, so validation matches stable TiDB.
- vitess dependency removed (and with it grpc/protobuf), closing the open
  grpc and vitess advisories; testify, x/sync, pterm and indirect deps
  bumped; toolchain go1.26.10.
- Removed the `strict_mode` and `preserve_comments` config keys: they were
  never implemented. Existing configs that set them still load.
- CI: actions bumped (checkout v7.0.1, setup-go v7, scorecard v2.5.0,
  codeql v4.39.1, goreleaser v7.3.0, go-test-coverage v2.20.0),
  golangci-lint v2.15.0, govulncheck v1.9.0; coverage badge is self-hosted
  on the `badges` branch (Codecov dropped).

## [1.1.0] - 2026-07-09

### Changed
- Releases are now published (not draft) with the checksums file signed via
  keyless cosign; workflow token permissions tightened to job level;
  govulncheck runs through the pinned official action; fuzz tests added for
  the SQL preprocessor and loader; SECURITY.md links the private reporting
  flow.
- Go raised to 1.26 (toolchain go1.26.5) alongside the vitess v0.24.2 bump;
  govulncheck clean.
- README badge row added (OpenSSF Scorecard, Go Reference, codecov, Go
  version, license); OpenSSF Scorecard workflow and CI coverage upload to
  Codecov.
- **KeyLength rule no longer truncates columns.** It now caps *index key
  prefix lengths* at 768 characters (TiDB's 3072-byte `max-index-length` in
  utf8mb4) instead of shrinking `VARCHAR` column definitions, which could
  silently truncate data on import. TiDB supports `VARCHAR` up to 16383
  characters.
- IndexPrefix rule now only adds the 255-character prefix to indexed
  TEXT/BLOB columns; it no longer adds redundant prefixes to VARCHAR/CHAR
  keys that already fit the limit.
- `enabled_rules` / `disabled_rules` from the config are now actually honored
  by the rule registry (previously ignored).
- JsonGenerated rule is disabled by default: modern TiDB (v6.3+) supports
  JSON functions in generated columns.
- FunctionDefault rule allowlist is configurable via
  `allowed_default_functions`; `localtime`/`localtimestamp` added to the
  defaults.
- `validate` command is now implemented: parses a file with the TiDB parser
  and reports errors/warnings.
- CLI: unimplemented stub commands (`extract`, `import`, `diff`) removed
  until they have real implementations.
- vitess bumped past GO-2026-4567 (v0.22.4, later v0.24.2);
  cobra/pterm/zap/x-sync updated.
- CI: gofmt check, race-enabled tests, govulncheck job, golangci-lint v2,
  actions pinned by commit SHA; goreleaser release workflow added.

### Added
- End-to-end golden tests (`test/golden_test.go`) covering the full
  load → transform → write pipeline; outputs are verified to re-parse with
  the TiDB parser.
- `scripts/migrate-mariadb-to-tidb.sh`: dump → transform → load pipeline
  wrapper (host-agnostic, passwords via `MYSQL_PWD`).
- `make vulncheck` target.

### Removed
- Stray scratch files at the repository root (`test_charset*.sql`,
  `test_config.yaml`) and dead stub rule code.

## [1.0.1] - 2025-12-06

### Changed
- Field collation fixes; CI/dependency maintenance.

## [0.1.0] - 2024-12-30

### Added
- Initial project bootstrap with CLI framework and rule-based transformer
