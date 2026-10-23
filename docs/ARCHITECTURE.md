# Architecture

## Overview

The mariadb2tidb tool is a command-line application that transforms MariaDB SQL schemas to be compatible with TiDB. The architecture follows a modular design with clear separation of concerns.

## Components

### CLI Layer (`cmd/mariadb2tidb/`)
- **main.go**: Entry point and CLI setup using the Cobra framework
- **commands.go**: Command definitions (`transform`, `transform-dir`, `validate`, `version`)

### Parser Layer (`internal/parser/`)
- **sqltok.go**: Lossless tokenizer (strings, quoted identifiers, comments, `/*! */` executable comments) and a script splitter that honors the mysql client's `DELIMITER`
- **preprocess.go**: Token-level rewrites of MariaDB-only syntax the TiDB parser rejects (see [RULES.md](RULES.md#loader-preprocessing-not-rules)); never touches string literals or comments
- **loader.go**: Loads SQL from files, strings, or readers: preprocess, then parse with the TiDB parser (parser panics become errors)
- **writer.go**: Converts the AST back to formatted SQL (TiDB parser restore, one column/constraint per line in CREATE TABLE; honors `formatting` config)
- **visitor.go**: AST traversal utilities with the visitor pattern

### Compatibility facts (`internal/compat/`)
- **collation.go**: Charsets and collations TiDB v8.5 accepts (verified against `information_schema` of a live server; the parser's own list differs) and deterministic resolution of source collations: configured exact names, then the most specific pattern, then a fallback within the charset
- **report.go**: Concurrency-safe collector of migration warnings, printed by the CLI

### Configuration (`internal/config/`)
- **config.go**: YAML-backed configuration: input/output directories, rule toggles, charset/collation mappings, allowed default functions

### Transformation Engine (`internal/transformer/`)
- **pipeline.go**: `TransformString`, the whole pipeline used by the CLI, the directory processor and the tests
- **engine.go**: Orchestrates rule application over the AST; a failing rule fails the transformation
- **shape.go**: Output check: tables, columns, keys and statement counts before vs after
- **dir_processor.go**: Mirrors a directory tree with parallel workers: schema `.sql` files are transformed, Dumpling data chunks and other files are copied unchanged

### Rules System (`internal/rules/`)
- **interface.go**: `Rule` interface definition
- **registry.go**: Rule registration, priority ordering, and enable/disable filtering
- One file per rule; see [RULES.md](RULES.md)

### Utilities (`internal/utils/`)
- **logger.go**: Structured logging using Zap

## Data Flow

1. **Input**: SQL file or string, e.g. `mariadb-dump` output (schema and data) or a Dumpling schema file
2. **Preprocess**: Split by `DELIMITER`, drop stored programs, token-level fixes for syntax the TiDB parser cannot represent (column types, attributes, table options, collations, `SETVAL`)
3. **Parse**: TiDB parser converts SQL to an AST; the shape (tables, columns, keys) is recorded
4. **Transform**: Rules engine applies transformation rules to the AST in priority order
5. **Format**: Writer restores the modified AST with the TiDB parser's own `Restore`
6. **Check**: The output is re-parsed and must have the recorded shape
7. **Output**: TiDB-compatible SQL plus a list of migration warnings

## Key Design Decisions

### AST-First Transformations
- Rules operate on the Abstract Syntax Tree, which guarantees syntactic correctness
- Preprocessing is kept to the minimum needed to make MariaDB-only syntax parseable at all, and works on tokens: string literals, comments and quoted identifiers are never rewritten
- Output is written only by the TiDB parser's `Restore`; a second parser or formatter (vitess was used before v1.2.0) can silently disagree with TiDB's grammar

### Rule-Based Architecture
- Each transformation is implemented as a rule with a priority
- Rules are idempotent and composable
- Rules can be toggled via `enabled_rules` / `disabled_rules` in the config

### Testing
- Unit tests per rule and for the tokenizer/preprocessor; fuzz tests for the tokenizer, preprocessor and loader
- End-to-end golden tests (`test/golden_test.go`) run every fixture through the full pipeline, verify the output re-parses with the TiDB parser, and compare against expected files (`go test ./test -run TestGolden -update` regenerates them)
- Integration tests (`test/integration`, build tag `integration`, `make integration-up integration`): every fixture is applied to a live TiDB in strict sql_mode, and a corpus of MariaDB features is created on a live MariaDB, dumped with `mariadb-dump`, converted, loaded into TiDB and compared: columns, order, nullability, indexes, every row byte for byte, sequence positions and collation behavior

## Future Enhancements

The architecture supports incremental development:
- Add new transformation rules to the registry
- Extend the CLI with additional commands
- Integration with TiDB ecosystem tools (DM task generation)
