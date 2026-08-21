#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

docker compose up -d postgres
docker compose exec -T postgres sh -c 'until psql -U orch -d orch -tAc "SELECT 1" >/dev/null 2>&1; do sleep 1; done'

if ! docker compose exec -T postgres psql -U orch -d orch -tAc "SELECT to_regclass('public.schema_migrations') IS NOT NULL" | grep -q t; then
  echo "No migration ledger found. Run scripts/migrate-up.sh first."
  exit 1
fi

version="$(docker compose exec -T postgres psql -U orch -d orch -tAc 'SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1' | tr -d '[:space:]')"
if [[ -z "$version" ]]; then
  echo "No applied migration to roll back."
  exit 0
fi

migration="$(find migrations -maxdepth 1 -name "${version}_*.down.sql" -print -quit)"
if [[ -z "$migration" ]]; then
  echo "Down migration for $version not found."
  exit 1
fi

docker compose exec -T postgres psql -U orch -d orch -v ON_ERROR_STOP=1 < "$migration"
docker compose exec -T postgres psql -U orch -d orch -v ON_ERROR_STOP=1 -c \
  "DELETE FROM schema_migrations WHERE version = '$version'" >/dev/null

echo "Rolled back migration $version."
