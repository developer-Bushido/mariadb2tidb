# Transformation Rules

## Overview

The mariadb2tidb tool uses a rule-based system to transform MariaDB SQL schemas into a TiDB-compatible format. Each rule addresses a specific compatibility difference, with the relevant TiDB documentation linked.

## Rule System

### Rule Interface
All rules implement the `Rule` interface:
```go
type Rule interface {
    Name() string                          // Unique rule identifier
    Description() string                   // Human-readable description
    Priority() int                         // Execution order (lower = earlier)
    ShouldApply(node ast.Node) bool        // Determine if rule applies to AST node
    Apply(node ast.Node) (ast.Node, error) // Transform the AST node
}
```

### Registry
- Rules run in priority order (lower number first)
- `enabled_rules` / `disabled_rules` in the YAML config control which rules are active; an empty `enabled_rules` list means "all rules"

## Active Rules

### Collation (Priority: 100)
Normalizes charsets and collations using the configurable `charset_mappings` / `collation_mappings`:
- `utf8mb4_unicode_*` → `utf8mb4_0900_ai_ci` (covers `utf8mb4_unicode_520_ci`, which TiDB does not support)
- `latin1_swedish_ci` → `utf8mb4_0900_ai_ci`
- `DEFAULT CHARSET=latin1` → `DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`

TiDB supports `utf8mb4_0900_ai_ci` as a client/default collation from **v7.4.0**; target older versions by overriding the mappings (e.g. to `utf8mb4_general_ci`).
Docs: <https://docs.pingcap.com/tidb/stable/character-set-and-collation>

### KeyLength (Priority: 300)
Caps **index key prefix lengths** at 768 characters (3072 bytes in utf8mb4, TiDB's default `max-index-length`). Applies to explicit oversized prefixes and to indexes over `CHAR`/`VARCHAR` columns wider than 768 characters. Column definitions are never truncated — TiDB supports `VARCHAR` up to 16383 characters.
Docs: <https://docs.pingcap.com/tidb/stable/tidb-limitations>

### IndexPrefix (Priority: 350)
Adds a 255-character prefix to indexed `TEXT`/`BLOB` columns that lack one; TiDB (like MySQL) cannot index these types without an explicit prefix.

### TextBlobDefaults (Priority: 400)
Removes `DEFAULT` values from `TEXT`/`BLOB`/`JSON` columns. TiDB rejects literal defaults on these types (expression defaults exist from v8.0.0, but stripping keeps output loadable on all supported versions).
Docs: <https://docs.pingcap.com/tidb/stable/data-type-default-values>

### JsonCheck (Priority: 500)
Removes `CHECK (JSON_VALID(...))` constraints that MariaDB emits for JSON columns (MariaDB stores JSON as `LONGTEXT` + check constraint).

### FunctionDefault (Priority: 600)
Removes function-based `DEFAULT` values not in the allowlist (`allowed_default_functions` in the config; defaults to the `CURRENT_TIMESTAMP` family: `current_timestamp`, `current_date`, `current_time`, `now`, `localtime`, `localtimestamp`). Extend the allowlist when targeting TiDB ≥ v8.0.0, which supports more default expressions.
Docs: <https://docs.pingcap.com/tidb/stable/data-type-default-values>

### JsonGenerated (Priority: 700) — disabled by default
Converts generated columns that use JSON functions into regular columns. Modern TiDB (v6.3+) supports JSON functions in generated columns, so this rule is only needed for legacy targets — enable it via `enabled_rules` or remove `JsonGenerated` from `disabled_rules`.
Docs: <https://docs.pingcap.com/tidb/stable/generated-columns>

## Loader Preprocessing (not rules)

Some MariaDB-only syntax cannot be represented in the TiDB parser AST, so the loader fixes it textually before parsing:
- `UUID` column type → `CHAR(36)` (function calls and identifiers named `uuid` are left alone); `NOT NULL` uuid columns get `DEFAULT ''`; unique keys named `uuid` are renamed to `uuid_key`
- Table encryption options (`ENCRYPTED=YES ENCRYPTION_KEY_ID=n`) are dropped
- Configured charset/collation mappings are applied textually as a safety net

## Rule Constraints

- Rules must be idempotent (applying twice = applying once)
- Rules should not modify nodes they don't handle
- Rules must preserve data: never shrink column types or drop columns
- Rules should be covered by unit tests **and** a golden fixture

## Adding New Rules

1. Implement the `Rule` interface in `internal/rules/`
2. Register it in `registerDefaultRules` (`registry.go`) with an appropriate priority
3. Add unit tests and a golden fixture (`test/fixtures/<name>.sql`, then `go test ./test -run TestGolden -update`)
4. Update this documentation

## Candidate Future Rules

- **ZeroTimestamp**: handle `'0000-00-00 00:00:00'` defaults (TiDB rejects them with default SQL mode)
- **AutoRandom**: optionally convert `AUTO_INCREMENT` primary keys to `AUTO_RANDOM` to avoid write hotspots
- **ShardRowID**: inject `SHARD_ROW_ID_BITS` for tables without a clustered primary key
- **Sequence/Trigger stripping**: MariaDB sequences and triggers are not supported by TiDB
