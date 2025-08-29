# Transformation Rules

## Overview

The mariadb2tidb tool uses a rule-based system to transform MariaDB SQL schemas into TiDB-compatible format. Each rule addresses a specific compatibility issue between MariaDB and TiDB.

## Rule System Architecture

### Rule Interface
All rules implement the `Rule` interface:
```go
type Rule interface {
    Name() string                          // Unique rule identifier
    Description() string                   // Human-readable description
    Priority() int                         // Execution order (lower = higher priority)
    ShouldApply(node ast.Node) bool       // Determine if rule applies to AST node
    Apply(node ast.Node) (ast.Node, error) // Transform the AST node
}
```

### Rule Registry
- Rules are registered in priority order
- Lower priority numbers execute first
- Rules are applied once per AST traversal

## Rules Implementation Status

### ✅ Completed Rules

### 1. Collation (Priority: 10) - T-0002
**Status**: ✅ Implemented
**Description**: Transform MariaDB collations to TiDB compatible ones
- `utf8mb4_unicode_*` → `utf8mb4_0900_ai_ci`
- `latin1_swedish_ci` → `utf8mb4_0900_ai_ci`
- `DEFAULT CHARSET=latin1` → `DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`

### 2. KeyLength (Priority: 300) - T-0004
**Status**: ✅ Implemented
**Description**: Cap oversized `VARCHAR` columns at 768 characters

### 3. TextBlobDefaults (Priority: 400)
**Status**: ✅ Implemented
**Description**: Remove default values from TEXT/BLOB/JSON columns

### 4. JsonCheck (Priority: 500)
**Status**: ✅ Implemented
**Description**: Remove JSON_VALID check constraints

### 5. FunctionDefault (Priority: 600)
**Status**: ✅ Implemented
**Description**: Remove unsupported function-based default values

### 6. JsonGenerated (Priority: 700)
**Status**: ✅ Implemented
**Description**: Convert JSON-based generated columns into regular columns

### 📋 Planned Rules (Implementation Order)

### 4. IntegerWidth (Priority: 400)
**Status**: Planned
**Description**: Remove integer display width specifications
- `INT(11)` → `INT`
- `BIGINT(20)` → `BIGINT`

### 7. ZeroTimestamp (Priority: 70)
**Status**: Planned
**Description**: Handle zero timestamp values
- Transform `'0000-00-00 00:00:00'` defaults

### 8. UUIDType (Priority: 80)
**Status**: Planned
**Description**: Transform UUID data types
- Map MariaDB UUID functions to TiDB equivalents

### 9. MariaDBSpecific (Priority: 90)
**Status**: Planned
**Description**: Remove MariaDB-specific syntax
- Storage engine options not supported by TiDB
- MariaDB-only features

### 10. Constraints (Priority: 100)
**Status**: Planned
**Description**: Transform constraint definitions
- Foreign key constraint handling
- Check constraint syntax differences

### 11. TrailingComma (Priority: 110)
**Status**: Planned
**Description**: Remove trailing commas
- Clean up syntax formatting

### 12. VersionMacros (Priority: 130)
**Status**: Planned
**Description**: Transform version-specific macros
- Handle conditional SQL based on database version

### 13. AutoIncrementValues (Priority: 140)
**Status**: Planned
**Description**: Adjust auto-increment starting values
- Handle differences in auto-increment behavior

### 14. OnUpdateCurrentTimestamp (Priority: 150)
**Status**: Planned
**Description**: Transform ON UPDATE CURRENT_TIMESTAMP
- Ensure proper timestamp update behavior

### 15. IndexType (Priority: 160)
**Status**: Planned
**Description**: Transform index type specifications
- Map MariaDB index types to TiDB equivalents

### 16. QualifiedNames (Priority: 170)
**Status**: Planned
**Description**: Handle qualified table/column names
- Ensure proper name resolution

## Rule Development Guidelines

### Implementation Pattern
1. Create rule struct implementing `Rule` interface
2. Register rule in registry with appropriate priority
3. Add unit tests for the rule
4. Add integration tests with legacy script comparison

### Testing Requirements
- Unit tests for rule logic
- Integration tests with fixture files
- Comparison with legacy script output
- Edge case handling

### Rule Constraints
- Rules must be idempotent (applying twice = applying once)
- Rules should not modify nodes they don't handle
- Rules must preserve SQL semantics
- Rules should provide clear error messages

## Adding New Rules

1. Implement the `Rule` interface
2. Add to `CreateStubRules()` in `stubs.go` (replace stub)
3. Register in priority order
4. Add comprehensive tests
5. Update this documentation

## Next Task: T-0003 KeyLength Rule

Based on AI_CONTEXT.yml task progression protocol, T-0003 should implement:

**KeyLength Rule (Priority: 20)**
- Transform KEY declarations with length > 768 characters  
- Add substring operations for varchar indexes per TiDB limits
- Handle both single and composite key constraints
- Reference: legacy script lines 50-52 in universal_tidb_transform.sh
- Files: `internal/rules/keylength.go`, tests, fixtures for various key scenarios 