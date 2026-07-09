#!/bin/bash
# MariaDB to TiDB migration pipeline: dump -> transform -> load.
# Uses the mariadb2tidb tool for schema transformation.
#
# Hosts and credentials come from flags or environment variables; nothing is
# hardcoded here. Passwords are passed to mysql/mysqldump via MYSQL_PWD so
# they never appear in the process list.
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Defaults (all overridable via flags or environment)
WORK_DIR="${WORK_DIR:-${TMPDIR:-/tmp}/mariadb2tidb-migration}"
MARIADB2TIDB_BIN="${MARIADB2TIDB_BIN:-mariadb2tidb}"
MARIADB_HOST="${MARIADB_HOST:-localhost}"
MARIADB_PORT="${MARIADB_PORT:-3306}"
MARIADB_USER="${MARIADB_USER:-root}"
MARIADB_PASS="${MARIADB_PASS:-}"
TIDB_HOST="${TIDB_HOST:-localhost}"
TIDB_PORT="${TIDB_PORT:-4000}"
TIDB_USER="${TIDB_USER:-root}"
TIDB_PASS="${TIDB_PASS:-}"

usage() {
    cat << EOF
Usage: $(basename "$0") [OPTIONS] COMMAND

MariaDB to TiDB migration pipeline built around the mariadb2tidb tool.

COMMANDS:
    dump        Extract schema/data from MariaDB
    transform   Transform SQL to TiDB-compatible format
    load        Load transformed SQL into TiDB
    full        Run complete pipeline (dump -> transform -> load)
    list-dbs    List databases on the MariaDB server

OPTIONS:
    -h, --mariadb-host      MariaDB host (default: localhost or \$MARIADB_HOST)
    -P, --mariadb-port      MariaDB port (default: 3306)
    -u, --mariadb-user      MariaDB user (default: root)
    -d, --database          Database name to migrate
    --tidb-host             TiDB host (default: localhost or \$TIDB_HOST)
    --tidb-port             TiDB port (default: 4000)
    --tidb-user             TiDB user (default: root)
    --schema-only           Dump schema only, no data
    --tables                Comma-separated list of tables to migrate
    --exclude-tables        Comma-separated list of tables to exclude
    --input                 Input file for transform/load
    -w, --work-dir          Working directory (default: $WORK_DIR)
    --dry-run               Show what would be done without executing
    --help                  Show this help

Passwords are never taken as flags: set MARIADB_PASS / TIDB_PASS in the
environment, or leave them empty to be prompted interactively.

EXAMPLES:
    # List databases
    $(basename "$0") -h db.example.internal -u readonly list-dbs

    # Dump schema, transform, and load into TiDB
    $(basename "$0") -h db.example.internal -u readonly -d mydb \\
        --tidb-host tidb.example.internal --schema-only full

    # Transform an existing dump file
    $(basename "$0") transform --input /path/to/dump.sql
EOF
}

SCHEMA_ONLY=false
TABLES=""
EXCLUDE_TABLES=""
DRY_RUN=false
INPUT_FILE=""
COMMAND=""
DATABASE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--mariadb-host)
            MARIADB_HOST="$2"; shift 2 ;;
        -P|--mariadb-port)
            MARIADB_PORT="$2"; shift 2 ;;
        -u|--mariadb-user)
            MARIADB_USER="$2"; shift 2 ;;
        -d|--database)
            DATABASE="$2"; shift 2 ;;
        --tidb-host)
            TIDB_HOST="$2"; shift 2 ;;
        --tidb-port)
            TIDB_PORT="$2"; shift 2 ;;
        --tidb-user)
            TIDB_USER="$2"; shift 2 ;;
        --schema-only)
            SCHEMA_ONLY=true; shift ;;
        --tables)
            TABLES="$2"; shift 2 ;;
        --exclude-tables)
            EXCLUDE_TABLES="$2"; shift 2 ;;
        -w|--work-dir)
            WORK_DIR="$2"; shift 2 ;;
        --input)
            INPUT_FILE="$2"; shift 2 ;;
        --dry-run)
            DRY_RUN=true; shift ;;
        --help)
            usage; exit 0 ;;
        dump|transform|load|full|list-dbs)
            COMMAND="$1"; shift ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1 ;;
    esac
done

mkdir -p "$WORK_DIR"

prompt_mariadb_password() {
    if [[ -z "$MARIADB_PASS" ]]; then
        read -rsp "Enter MariaDB password for $MARIADB_USER@$MARIADB_HOST: " MARIADB_PASS
        echo
    fi
}

# Run mysql against MariaDB; password travels via MYSQL_PWD, not argv.
mariadb_client() {
    MYSQL_PWD="$MARIADB_PASS" mysql -h "$MARIADB_HOST" -P "$MARIADB_PORT" -u "$MARIADB_USER" "$@"
}

# Run mysql against TiDB.
tidb_client() {
    MYSQL_PWD="$TIDB_PASS" mysql -h "$TIDB_HOST" -P "$TIDB_PORT" -u "$TIDB_USER" "$@"
}

cmd_list_dbs() {
    prompt_mariadb_password
    log_info "Listing databases on $MARIADB_HOST:$MARIADB_PORT..."

    if $DRY_RUN; then
        log_info "[DRY RUN] Would run: mysql -h $MARIADB_HOST -P $MARIADB_PORT -u $MARIADB_USER -N -e 'SHOW DATABASES'"
        return
    fi
    mariadb_client -N -e 'SHOW DATABASES' | grep -vE '^(information_schema|performance_schema|mysql|sys)$'
}

cmd_dump() {
    if [[ -z "$DATABASE" ]]; then
        log_error "Database name required (-d/--database)"
        exit 1
    fi

    prompt_mariadb_password

    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local dump_file="$WORK_DIR/${DATABASE}_${timestamp}.sql"

    local args=(--single-transaction --routines --triggers --events --skip-lock-tables)
    $SCHEMA_ONLY && args+=(--no-data)

    if [[ -n "$TABLES" ]]; then
        read -ra table_list <<< "${TABLES//,/ }"
        args+=("$DATABASE" "${table_list[@]}")
    elif [[ -n "$EXCLUDE_TABLES" ]]; then
        for table in ${EXCLUDE_TABLES//,/ }; do
            args+=("--ignore-table=$DATABASE.$table")
        done
        args+=("$DATABASE")
    else
        args+=("$DATABASE")
    fi

    log_info "Dumping $DATABASE from $MARIADB_HOST:$MARIADB_PORT (schema only: $SCHEMA_ONLY)..."
    log_info "Output: $dump_file"

    if $DRY_RUN; then
        log_info "[DRY RUN] Would run: mysqldump ${args[*]} > $dump_file"
        return
    fi

    MYSQL_PWD="$MARIADB_PASS" mysqldump -h "$MARIADB_HOST" -P "$MARIADB_PORT" -u "$MARIADB_USER" \
        "${args[@]}" > "$dump_file"
    log_success "Dump completed: $dump_file ($(du -h "$dump_file" | cut -f1))"
    echo "$dump_file"
}

cmd_transform() {
    local input="$INPUT_FILE"

    # If no input file specified, look for the latest dump
    if [[ -z "$input" && -n "$DATABASE" ]]; then
        input=$(find "$WORK_DIR" -maxdepth 1 -name "${DATABASE}_*.sql" ! -name '*_tidb.sql' -print0 2>/dev/null \
            | xargs -0 ls -t 2>/dev/null | head -1)
    fi
    if [[ -z "$input" ]]; then
        log_error "No input file. Use --input or specify --database with an existing dump"
        exit 1
    fi
    if [[ ! -f "$input" ]]; then
        log_error "Input file not found: $input"
        exit 1
    fi

    local output="${input%.sql}_tidb.sql"
    log_info "Transforming $input -> $output"

    if $DRY_RUN; then
        log_info "[DRY RUN] Would run: $MARIADB2TIDB_BIN transform '$input' -o '$output'"
        return
    fi

    if ! command -v "$MARIADB2TIDB_BIN" &> /dev/null; then
        log_warn "mariadb2tidb not in PATH, trying to build..."
        local script_dir project_dir
        script_dir="$(cd "$(dirname "$0")" && pwd)"
        project_dir="$(dirname "$script_dir")"
        if [[ -f "$project_dir/Makefile" ]]; then
            make -C "$project_dir" build
            MARIADB2TIDB_BIN="$project_dir/bin/mariadb2tidb"
        else
            log_error "Cannot find mariadb2tidb binary"
            exit 1
        fi
    fi

    "$MARIADB2TIDB_BIN" transform "$input" -o "$output"
    log_success "Transform completed: $output ($(du -h "$output" | cut -f1))"
    echo "$output"
}

cmd_load() {
    local input="$INPUT_FILE"

    if [[ -z "$input" && -n "$DATABASE" ]]; then
        input=$(find "$WORK_DIR" -maxdepth 1 -name "${DATABASE}_*_tidb.sql" -print0 2>/dev/null \
            | xargs -0 ls -t 2>/dev/null | head -1)
    fi
    if [[ -z "$input" ]]; then
        log_error "No input file. Use --input or run transform first"
        exit 1
    fi
    if [[ ! -f "$input" ]]; then
        log_error "Input file not found: $input"
        exit 1
    fi

    local db_name="${DATABASE:-$(basename "${input%_*_tidb.sql}")}"
    log_info "Loading $input into TiDB at $TIDB_HOST:$TIDB_PORT (database: $db_name)..."

    if $DRY_RUN; then
        log_info "[DRY RUN] Would run: mysql -h $TIDB_HOST -P $TIDB_PORT -u $TIDB_USER $db_name < '$input'"
        return
    fi

    tidb_client -e "CREATE DATABASE IF NOT EXISTS \`$db_name\`"
    tidb_client "$db_name" < "$input"
    log_success "Load completed!"

    local table_count
    table_count=$(tidb_client -N -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$db_name'")
    log_info "Tables in $db_name: $table_count"
}

cmd_full() {
    log_info "Starting full migration pipeline..."
    log_info "MariaDB: $MARIADB_HOST:$MARIADB_PORT/$DATABASE"
    log_info "TiDB: $TIDB_HOST:$TIDB_PORT"
    echo

    log_info "=== Step 1: Dump from MariaDB ==="
    local dump_file
    dump_file=$(cmd_dump | tail -1)
    [[ -z "$dump_file" ]] && exit 1
    INPUT_FILE="$dump_file"
    echo

    log_info "=== Step 2: Transform to TiDB format ==="
    local transform_file
    transform_file=$(cmd_transform | tail -1)
    [[ -z "$transform_file" ]] && exit 1
    INPUT_FILE="$transform_file"
    echo

    log_info "=== Step 3: Load into TiDB ==="
    cmd_load
    echo

    log_success "Migration completed successfully!"
    log_info "Dump: $dump_file"
    log_info "Transformed: $transform_file"
}

if [[ -z "$COMMAND" ]]; then
    log_error "Command required"
    usage
    exit 1
fi

case "$COMMAND" in
    list-dbs)  cmd_list_dbs ;;
    dump)      cmd_dump ;;
    transform) cmd_transform ;;
    load)      cmd_load ;;
    full)      cmd_full ;;
    *)
        log_error "Unknown command: $COMMAND"
        usage
        exit 1 ;;
esac
