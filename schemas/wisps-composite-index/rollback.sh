#!/usr/bin/env bash
#
# Drops idx_wisps_type_status_assignee from the Dolt-backed beads wisps table
# and commits the rollback to Dolt history. Uses the same connection discovery
# documented in migrate.sh.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/common.sh"

resolve_connection
ensure_connection
ensure_database_and_table

info "using Dolt database $DOLT_DB on $DOLT_HOST:$DOLT_PORT as $DOLT_USER"

indexes=$(show_wisps_indexes)
rows=$(index_rows "$indexes")
if [ "$rows" -eq 0 ]; then
    info "index $INDEX_NAME is absent; no rollback changes needed"
    exit 0
fi

verify_index_definition "$indexes"

dolt_sql -q "
    USE \`$DOLT_DB\`;
    DROP INDEX $INDEX_NAME ON wisps;
" >/dev/null

indexes=$(show_wisps_indexes)
rows=$(index_rows "$indexes")
if [ "$rows" -ne 0 ]; then
    die "rollback failed; index $INDEX_NAME is still present"
fi

commit_schema_change "schema: drop $INDEX_NAME from wisps" >/dev/null
info "dropped and committed $INDEX_NAME from wisps"
