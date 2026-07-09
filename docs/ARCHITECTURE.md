# Architecture

## Overview

The mariadb2tidb tool is a command-line application that transforms MariaDB SQL schemas to be compatible with TiDB. The architecture follows a modular design with clear separation of concerns.

## Components

### CLI Layer (`cmd/mariadb2tidb/`)
- **main.go**: Entry point and CLI setup using the Cobra framework
- **commands.go**: Command definitions (`transform`, `transform-dir`, `validate`, `version`)

### Parser Layer (`internal/parser/`)
- **loader.go**: Loads SQL from files, strings, or readers using the TiDB parser; applies light textual preprocessing for constructs the parser rejects (MariaDB `UUID` type, table encryption options, charset/collation mappings)
- **writer.go**: Converts the AST back to formatted SQL (TiDB restore + vitess pretty-printing)
- **visitor.go**: AST traversal utilities with the visitor pattern

### Configuration (`internal/config/`)
- **config.go**: YAML-backed configuration: input/output directories, rule toggles, charset/collation mappings, allowed default functions

### Transformation Engine (`internal/transformer/`)
- **engine.go**: Orchestrates rule application over the AST
- **dir_processor.go**: Applies the pipeline to a directory tree with parallel workers

### Rules System (`internal/rules/`)
- **interface.go**: `Rule` interface definition
- **registry.go**: Rule registration, priority ordering, and enable/disable filtering
- One file per rule; see [RULES.md](RULES.md)

### Utilities (`internal/utils/`)
- **logger.go**: Structured logging using Zap

## Data Flow

1. **Input**: SQL file or string containing a MariaDB schema
2. **Preprocess**: Targeted text fixes for syntax the TiDB parser cannot represent (UUID columns, `ENCRYPTED=YES` options, charset mappings)
3. **Parse**: TiDB parser converts SQL to an AST
4. **Transform**: Rules engine applies transformation rules to the AST in priority order
5. **Format**: Writer restores the modified AST back into readable SQL
6. **Output**: TiDB-compatible SQL

## Key Design Decisions

### AST-First Transformations
- Rules operate on the Abstract Syntax Tree, which guarantees syntactic correctness
- Text preprocessing is kept to the minimum needed to make MariaDB-only syntax parseable at all

### Rule-Based Architecture
- Each transformation is implemented as a rule with a priority
- Rules are idempotent and composable
- Rules can be toggled via `enabled_rules` / `disabled_rules` in the config

### Testing
- Unit tests per rule
- End-to-end golden tests (`test/golden_test.go`) run every fixture through the full pipeline, verify the output re-parses with the TiDB parser, and compare against expected files (`go test ./test -run TestGolden -update` regenerates them)

## Future Enhancements

The architecture supports incremental development:
- Add new transformation rules to the registry
- Extend the CLI with additional commands
- Add support for data migration (not just schema)
- Integration with TiDB ecosystem tools (Dumpling, Lightning, DM)
