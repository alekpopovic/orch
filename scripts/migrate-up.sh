#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

docker compose up -d postgres
docker compose exec -T postgres sh -c 'until pg_isready -U orch -d orch >/dev/null 2>&1; do sleep 1; done'
if docker compose exec -T postgres psql -U orch -d orch -tAc "SELECT to_regclass('public.nodes') IS NOT NULL" | grep -q t; then
  echo "Initial schema already exists; skipping migrations."
  exit 0
fi
docker compose exec -T postgres psql -U orch -d orch -v ON_ERROR_STOP=1 < migrations/000001_initial_schema.up.sql

echo "Applied migrations."
