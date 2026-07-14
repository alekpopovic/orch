#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/backup-db.sh --database-url <postgres-url> [--output <backup.dump>]

Creates a PostgreSQL custom-format dump with pg_dump.

Required:
  --database-url   PostgreSQL connection URL. No default is used.

Optional:
  --output         Output dump path. Defaults to backups/orch-<UTC timestamp>.dump.
USAGE
}

database_url=""
output=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --database-url)
      database_url="${2:-}"
      shift 2
      ;;
    --output)
      output="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$database_url" ]]; then
  echo "--database-url is required" >&2
  usage >&2
  exit 2
fi

if ! command -v pg_dump >/dev/null 2>&1; then
  echo "pg_dump is required but was not found in PATH" >&2
  exit 1
fi

if [[ -z "$output" ]]; then
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  output="backups/orch-${timestamp}.dump"
fi

mkdir -p "$(dirname "$output")"

pg_dump \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file "$output" \
  "$database_url"

echo "Backup written to $output"
