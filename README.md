# mariadb2tidb

[![CI](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml/badge.svg)](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/developer-Bushido/mariadb2tidb/badge)](https://scorecard.dev/viewer/?uri=github.com/developer-Bushido/mariadb2tidb)
[![Go Reference](https://pkg.go.dev/badge/github.com/developer-Bushido/mariadb2tidb.svg)](https://pkg.go.dev/github.com/developer-Bushido/mariadb2tidb)
[![Coverage](https://raw.githubusercontent.com/developer-Bushido/mariadb2tidb/badges/.badges/main/coverage.svg)](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/developer-Bushido/mariadb2tidb)](go.mod)
[![License](https://img.shields.io/github/license/developer-Bushido/mariadb2tidb)](LICENSE)

A command-line tool that converts MariaDB schemas into TiDB-compatible SQL.

## Features

- Reads real `mariadb-dump` output (MariaDB 10.x–11.x, schema and data):
  `DELIMITER` blocks, executable comments, MariaDB 11 `uca1400` collations,
  `UUID`/`INET4`/`INET6`/spatial types, MariaDB-only attributes and table
  options, sequences with their current value, generated-column values
- Rule-based schema transformation to the TiDB dialect (collations, engines,
  index key length limits, TEXT/BLOB defaults, `JSON_VALID` checks,
  `JSON_VALUE`, zero dates, optional `AUTO_RANDOM` / `SHARD_ROW_ID_BITS` — see
  [docs/RULES.md](docs/RULES.md))
- A list of migration warnings for everything that changed meaning or was
  dropped (stored programs, invisible columns, system versioning, …)
- Output check: the result must still contain every table, column, key and
  statement of the input, or the run fails
- Directory mode for Dumpling exports: schema files are transformed, data
  chunks and other files are copied unchanged
- Tested end to end against live MariaDB 11.4 and 11.8 and TiDB v8.5.8 and
  v7.5.8 (schema, every row byte for byte, sequences, collations); defaults
  target TiDB ≥ v7.4 (configurable)

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

A full `mariadb-dump` (schema and data) works as is:

```bash
mariadb-dump --routines --triggers --events --databases shop > shop.sql
mariadb2tidb transform shop.sql -o shop_tidb.sql
mysql -h tidb -P 4000 -u root < shop_tidb.sql
```

Transform a Dumpling output directory into a TiDB Lightning source:
schema `.sql` files are transformed, data chunks (`db.table.000000000.sql`
from Dumpling, `db.table.00000.sql` from mydumper, CSV, compressed chunks) and
other files such as `metadata` are copied unchanged. Output files are written
to a temporary file and renamed into place, so links left in the output
directory are replaced, never written through.

```bash
mariadb2tidb transform ./DUMPLING -o ./LIGHTNING --workers 8
```

Both print a list of migration warnings at the end: dropped triggers and
routines, columns that are no longer invisible, replaced engines, zero-date
defaults and similar changes that need a human decision.

The output directory must not be the input directory or nested with it, and
symbolic links inside the input tree are rejected rather than skipped.
Gzip-compressed schema files are transformed and recompressed; zstd/snappy
schema files must be decompressed first.

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
zero_date_defaults: keep           # or drop: TiDB rejects them in strict sql_mode
auto_random:                       # opt-in AUTO_INCREMENT -> AUTO_RANDOM
  tables: []                       # table names or ["*"]
  shard_bits: 5
  range_bits: 54                   # IDs stay below 2^53 (JavaScript, PHP)
shard_row_id_bits: 0               # >0: SHARD_ROW_ID_BITS for tables without a clustered PK
```

Unknown keys and values TiDB would reject (for example `shard_bits` outside
1–15) are reported as errors instead of falling back to defaults.

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
- Integration tests against live MariaDB and TiDB containers (Docker):
  ```bash
  make integration-up integration integration-down
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
