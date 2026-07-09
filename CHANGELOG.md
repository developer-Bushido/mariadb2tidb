# Changelog

All notable changes to the mariadb2tidb project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
