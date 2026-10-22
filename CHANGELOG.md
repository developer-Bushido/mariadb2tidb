# Changelog

All notable changes to the mariadb2tidb project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- Output files and stdout now end with a trailing newline.
- Preprocessor no longer panics (or inserts `default ''` at a wrong offset)
  on input with invalid UTF-8 or length-changing letters near a `char(36)`
  column; found by fuzzing.
- A TiDB parser panic (e.g. a decimal literal over 81 digits) is reported as
  a normal load/validate error instead of crashing the run.

### Changed
- Output format: uppercase keywords, every identifier backtick-quoted,
  2-space indent, table options as TiDB writes them
  (`DEFAULT CHARACTER SET = UTF8MB4`).
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
