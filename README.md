# mariadb2tidb

[![CI](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml/badge.svg)](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/developer-Bushido/mariadb2tidb/badge)](https://scorecard.dev/viewer/?uri=github.com/developer-Bushido/mariadb2tidb)
[![Go Report Card](https://goreportcard.com/badge/github.com/developer-Bushido/mariadb2tidb)](https://goreportcard.com/report/github.com/developer-Bushido/mariadb2tidb)
[![Go Reference](https://pkg.go.dev/badge/github.com/developer-Bushido/mariadb2tidb.svg)](https://pkg.go.dev/github.com/developer-Bushido/mariadb2tidb)
[![codecov](https://codecov.io/gh/developer-Bushido/mariadb2tidb/graph/badge.svg)](https://codecov.io/gh/developer-Bushido/mariadb2tidb)
[![Go Version](https://img.shields.io/github/go-mod/go-version/developer-Bushido/mariadb2tidb)](go.mod)
[![License](https://img.shields.io/github/license/developer-Bushido/mariadb2tidb)](LICENSE)

A command-line tool that converts MariaDB schemas into TiDB-compatible SQL.

## Features

- Rule-based schema transformation to the TiDB dialect (collations, index key
  length limits, TEXT/BLOB defaults, `JSON_VALID` checks, UUID columns — see
  [docs/RULES.md](docs/RULES.md))
- Recursive directory transformation with parallel workers
- Output validation against the TiDB parser
- Rules verified against the [TiDB documentation](https://docs.pingcap.com/tidb/stable/);
  defaults target TiDB ≥ v7.4 (configurable)

## Installation

Requires Go 1.26+.

Quick install (latest):
```bash
go install github.com/developer-Bushido/mariadb2tidb/cmd/mariadb2tidb@latest
```

From source:
```bash
git clone https://github.com/developer-Bushido/mariadb2tidb.git
cd mariadb2tidb
make build
./bin/mariadb2tidb version
```

## Usage

Transform a MariaDB schema dump:

```bash
mariadb2tidb transform schema.sql -o schema_tidb.sql
```

Transform a directory tree of `.sql` files (e.g. a Dumpling output directory):

```bash
mariadb2tidb transform ./DUMPLING -o ./LIGHTNING --workers 8
```

Validate that a file parses with the TiDB parser:

```bash
mariadb2tidb validate schema_tidb.sql
```

See `mariadb2tidb --help` for all commands, and
[scripts/migrate-mariadb-to-tidb.sh](scripts/migrate-mariadb-to-tidb.sh) for a
full dump → transform → load pipeline wrapper.

### Configuration file

Run commands with `--config` to override the defaults
([config/default.yaml](config/default.yaml)):

```yaml
input_dir: ./DUMPLING
output_dir: ./LIGHTNING
enabled_rules: []                  # empty = all rules
disabled_rules: [JsonGenerated]    # modern TiDB supports JSON generated columns
charset_mappings:
  latin1:
    target_charset: utf8mb4
    target_collation: utf8mb4_0900_ai_ci
collation_mappings:
  latin1_swedish_ci: utf8mb4_0900_ai_ci
  utf8mb4_unicode_*: utf8mb4_0900_ai_ci
allowed_default_functions:         # kept in DEFAULT clauses; others stripped
  - current_timestamp
  - now
```

Targeting TiDB older than v7.4? Map collations to `utf8mb4_general_ci`
instead of `utf8mb4_0900_ai_ci`.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Transformation rules](docs/RULES.md)

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development
- Requirements: Go 1.26+ (toolchain pinned in go.mod).
- Format/Vet/Test:
  ```bash
  make fmt vet test
  ```
- Lint (golangci-lint v2):
  ```bash
  golangci-lint run
  ```
- Vulnerability scan:
  ```bash
  make vulncheck
  ```
- Regenerate golden test files after intentional behavior changes:
  ```bash
  go test ./test -run TestGolden -update
  ```
- Build with version metadata:
  ```bash
  make build   # uses git tag/commit and date via ldflags
  ```

## License

mariadb2tidb is licensed under the [Apache 2.0](LICENSE) license.

## Code of Conduct

This project follows a [Code of Conduct](CODE_OF_CONDUCT.md). Please read it to understand the expected behavior.

## Security

If you discover a security vulnerability, please follow our [Security Policy](SECURITY.md) and report it privately via GitHub Security Advisories.
