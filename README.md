# mariadb2tidb [![CI](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml/badge.svg)](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml)

A command-line tool that converts MariaDB schemas into TiDB-compatible SQL.

## Features

- Schema transformation to TiDB dialect
- Database extraction from multi-DB dumps
- Parallel import to TiDB
- Validation and diff generation

## Installation

Requires Go 1.24+.

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

Transform a MariaDB schema:

```bash
mariadb2tidb transform schema.sql -o schema_tidb.sql
```

You can also define default input and output directories in a YAML config file:

```yaml
input_dir: ./DUMPLING
output_dir: ./LIGHTNING
```

Run commands with `--config` to read these paths.

See `mariadb2tidb --help` for all commands.

### Configuration file
Minimal `config/default.yaml` example:
```yaml
input_dir: ./DUMPLING
output_dir: ./LIGHTNING
enabled_rules: []
disabled_rules: []
strict_mode: true
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Transformation rules](docs/RULES.md)

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development
- Requirements: Go 1.24+.
- Format/Vet/Test:
  ```bash
  make fmt vet test
  ```
- Lint (via golangci-lint):
  ```bash
  golangci-lint run
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
