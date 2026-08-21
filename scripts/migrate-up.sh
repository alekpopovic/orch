#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

docker compose up -d postgres
docker compose exec -T postgres sh -c 'until psql -U orch -d orch -tAc "SELECT 1" >/dev/null 2>&1; do sleep 1; done'

docker compose exec -T postgres psql -U orch -d orch -v ON_ERROR_STOP=1 -c \
  "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()))" >/dev/null

migration_present() {
  local version="$1"
  local check="false"
  case "$version" in
    000001) check="to_regclass('public.nodes') IS NOT NULL" ;;
    000002) check="EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='tasks' AND column_name='ports')" ;;
    000003) check="EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='services' AND column_name='routes')" ;;
    000004) check="to_regclass('public.secrets') IS NOT NULL" ;;
    000005) check="to_regclass('public.registry_credentials') IS NOT NULL" ;;
    000006) check="EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='tasks' AND column_name='conditions')" ;;
    000007) check="EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='events' AND column_name='details')" ;;
    000008) check="to_regclass('public.audit_logs') IS NOT NULL" ;;
    000009) check="EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='services' AND column_name='security_context')" ;;
    000010) check="EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='services' AND column_name='autoscaling')" ;;
    000011) check="to_regclass('public.namespaces') IS NOT NULL" ;;
  esac
  docker compose exec -T postgres psql -U orch -d orch -tAc "SELECT $check" | grep -q t
}

for migration in migrations/*.up.sql; do
  filename="$(basename "$migration")"
  version="${filename%%_*}"
  if docker compose exec -T postgres psql -U orch -d orch -tAc \
    "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '$version')" | grep -q t; then
    continue
  fi
  if migration_present "$version"; then
    docker compose exec -T postgres psql -U orch -d orch -v ON_ERROR_STOP=1 -c \
      "INSERT INTO schema_migrations (version) VALUES ('$version') ON CONFLICT DO NOTHING" >/dev/null
    echo "Recorded existing migration $version."
    continue
  fi
  docker compose exec -T postgres psql -U orch -d orch -v ON_ERROR_STOP=1 < "$migration"
  docker compose exec -T postgres psql -U orch -d orch -v ON_ERROR_STOP=1 -c \
    "INSERT INTO schema_migrations (version) VALUES ('$version')" >/dev/null
  echo "Applied migration $version."
done

echo "All migrations are up to date."
