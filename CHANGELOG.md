# Changelog

All notable changes to the mariadb2tidb project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
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
- Toolchain pinned to Go 1.25.12; vitess bumped to v0.22.4 (fixes
  GO-2026-4567); cobra/pterm/zap/x-sync updated.
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
