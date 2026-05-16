#!/bin/sh
# gc dolt cleanup - forwarder shim to the Go runner.
#
# The destructive Dolt cleanup logic now lives in `gc dolt-cleanup`
# (Go, see cmd/gc/cmd_dolt_cleanup.go). This script translates the
# historical argv (--force, --max N, --server-down-ok) into the
# Go runner's flags and execs it. Backward compat for stale operator
# scripts (NFR-04 of ga-nw4z6); no classification, no DROP, no rm.
set -e

args=""
while [ $# -gt 0 ]; do
  case "$1" in
    --force)           args="$args --force"; shift ;;
    --max)             args="$args --max-orphan-dbs $2"; shift 2 ;;
    --server-down-ok)  echo "gc dolt cleanup: --server-down-ok is no longer supported; the SQL DROP path is the sole deletion mechanism" >&2; shift ;;
    --probe|--json)    args="$args $1"; shift ;;
    -h|--help)         exec gc dolt-cleanup --help ;;
    *)                 args="$args $1"; shift ;;
  esac
done

if [ "${GC_CLEANUP_JSON:-0}" = "1" ]; then
  case " $args " in
    *" --json "*) ;;
    *) args="$args --json" ;;
  esac
fi

# shellcheck disable=SC2086  # $args is whitespace-separated by design
exec gc dolt-cleanup $args
