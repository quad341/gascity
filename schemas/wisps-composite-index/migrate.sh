#!/usr/bin/env bash
#
# Adds idx_wisps_type_status_assignee to the Dolt-backed beads wisps table.
#
# Connection discovery order:
#   database: GC_DOLT_DATABASE, BEADS_DOLT_DATABASE, then .beads/metadata.json dolt_database
#   host:     GC_DOLT_HOST, BEADS_DOLT_SERVER_HOST, then 127.0.0.1
#   port:     GC_DOLT_PORT, BEADS_DOLT_SERVER_PORT, .beads/dolt-server.port,
#             .beads/config.yaml dolt.port, then GC runtime Dolt state JSON
#   user:     GC_DOLT_USER, BEADS_DOLT_SERVER_USER, then root
#   password: GC_DOLT_PASSWORD, BEADS_DOLT_PASSWORD, then empty
#
# Rollback path: run ./rollback.sh from this directory against the same
# connection settings.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/common.sh"

resolve_connection
ensure_connection
ensure_database_and_table

info "using Dolt database $DOLT_DB on $DOLT_HOST:$DOLT_PORT as $DOLT_USER"

indexes=$(show_wisps_indexes)
rows=$(index_rows "$indexes")
if [ "$rows" -gt 0 ]; then
    verify_index_definition "$indexes"
    info "index $INDEX_NAME already exists on wisps($INDEX_COLUMNS); no changes needed"
    exit 0
fi

dolt_sql -q "
    USE \`$DOLT_DB\`;
    CREATE INDEX $INDEX_NAME ON wisps(issue_type, status, assignee);
" >/dev/null

indexes=$(show_wisps_indexes)
verify_index_definition "$indexes"

commit_schema_change "schema: add $INDEX_NAME on wisps" >/dev/null
info "created and committed $INDEX_NAME on wisps($INDEX_COLUMNS)"
