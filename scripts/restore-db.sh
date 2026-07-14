#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/restore-db.sh --database-url <postgres-url> --input <backup.dump|backup.sql> --yes [--clean]

Restores an orch PostgreSQL backup.

Required:
  --database-url   PostgreSQL connection URL. No default is used.
  --input          Backup file created by scripts/backup-db.sh or a plain SQL dump.
  --yes            Explicit acknowledgement that restore changes the target database.

Optional:
  --clean          Drop restored objects before recreating them. Destructive.
USAGE
}

database_url=""
input=""
clean=false
yes=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --database-url)
      database_url="${2:-}"
      shift 2
      ;;
    --input)
      input="${2:-}"
      shift 2
      ;;
    --clean)
      clean=true
      shift
      ;;
    --yes)
      yes=true
      shift
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

if [[ -z "$input" ]]; then
  echo "--input is required" >&2
  usage >&2
  exit 2
fi

if [[ ! -f "$input" ]]; then
  echo "backup file not found: $input" >&2
  exit 1
fi

if [[ "$yes" != true ]]; then
  cat >&2 <<'WARNING'
Refusing to restore without --yes.

Restore modifies the target database. If --clean is also supplied, existing
objects in the target database may be dropped before being recreated.
WARNING
  exit 2
fi

if [[ "$clean" == true ]]; then
  cat >&2 <<'WARNING'
WARNING: --clean is destructive.
The restore will ask PostgreSQL to drop matching existing objects before
recreating them from the backup.
WARNING
fi

has_pg_restore=false
if command -v pg_restore >/dev/null 2>&1; then
  has_pg_restore=true
fi

if [[ "$has_pg_restore" == true ]] && pg_restore --list "$input" >/dev/null 2>&1; then
  args=(--no-owner --no-privileges --dbname "$database_url")
  if [[ "$clean" == true ]]; then
    args=(--clean --if-exists "${args[@]}")
  fi
  pg_restore "${args[@]}" "$input"
else
  if ! command -v psql >/dev/null 2>&1; then
    echo "psql is required but was not found in PATH" >&2
    exit 1
  fi
  if [[ "$clean" == true ]]; then
    echo "--clean is only supported for pg_dump custom-format backups; restore plain SQL into an empty database or include clean statements in the dump" >&2
    exit 2
  fi
  psql -v ON_ERROR_STOP=1 "$database_url" -f "$input"
fi

echo "Restore completed from $input"
