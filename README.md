# mariadb2tidb [![CI](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml/badge.svg)](https://github.com/developer-Bushido/mariadb2tidb/actions/workflows/ci.yml)

A command-line tool that converts MariaDB schemas into TiDB-compatible SQL.

## Features

- Schema transformation to TiDB dialect
- Database extraction from multi-DB dumps
- Parallel import to TiDB
- Validation and diff generation

## Installation

Requires Go 1.24+.

```bash
git clone https://github.com/developer-Bushido/mariadb2tidb.git
cd mariadb2tidb
make build
```

The binary will be created at `bin/mariadb2tidb`.

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

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Transformation rules](docs/RULES.md)

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

mariadb2tidb is licensed under the [Apache 2.0](LICENSE) license.

## Code of Conduct

This project follows a [Code of Conduct](CODE_OF_CONDUCT.md). Please read it to understand the expected behavior.

## Security

If you discover a security vulnerability, please follow our [Security Policy](SECURITY.md) and report it privately via GitHub Security Advisories.
