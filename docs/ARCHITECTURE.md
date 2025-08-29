# Architecture

## Overview

The mariadb2tidb tool is designed as a command-line application that transforms MariaDB SQL schemas to be compatible with TiDB. The architecture follows a modular design pattern with clear separation of concerns.

## Components

### CLI Layer (`cmd/mariadb2tidb/`)
- **main.go**: Entry point and CLI setup using Cobra framework
- **commands.go**: Command definitions (transform, extract, import, validate, diff)

### Parser Layer (`internal/parser/`)
- **loader.go**: Loads SQL from files, strings, or readers using TiDB parser
- **writer.go**: Converts AST back to formatted SQL with proper indentation
- **visitor.go**: AST traversal utilities with visitor pattern
- **test_helper.go**: Testing utilities for parser components

### Transformation Engine (`internal/transformer/`)
- **engine.go**: Orchestrates rule application and AST transformation
- **config.go**: Configuration management for transformation options

### Rules System (`internal/rules/`)
- **interface.go**: Rule interface definition
- **registry.go**: Rule registration and management
- **stubs.go**: Placeholder implementations for future rules

### Utilities (`internal/utils/`)
- **logger.go**: Structured logging using Zap

### Legacy Scripts (historical)
- Note: Legacy shell scripts have been removed from the public repository. They are referenced here only for historical context — the Go implementation supersedes them.

## Data Flow

1. **Input**: SQL file or string containing MariaDB schema
2. **Parse**: TiDB parser converts SQL to AST
3. **Transform**: Rules engine applies transformation rules to AST
4. **Format**: Writer converts modified AST back to readable SQL
5. **Output**: TiDB-compatible SQL

## Key Design Decisions

### AST-Only Transformations
- All transformations operate on the Abstract Syntax Tree
- No regex-based text manipulation
- Ensures syntactic correctness

### Rule-Based Architecture
- Each transformation is implemented as a rule
- Rules are applied in priority order
- Rules are idempotent and composable

### Test-Driven Development
- Legacy scripts serve as oracle for expected behavior
- Unit tests for individual components
- Integration tests for end-to-end workflows

### Modular Design
- Clear separation between parsing, transformation, and formatting
- Each component can be tested independently
- Easy to extend with new rules or features

## Future Enhancements

The architecture supports incremental development:
- Add new transformation rules to the registry
- Extend CLI with additional commands
- Add support for data migration (not just schema)
- Integration with TiDB ecosystem tools 
