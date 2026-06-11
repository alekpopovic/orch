#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Created .env from .env.example"
fi

docker compose --profile local-orch up --build -d postgres orch-server orch-agent

echo "Waiting for orch-server..."
until curl -fsS http://localhost:8080/readyz >/dev/null; do
  sleep 1
done

echo "Local environment is starting:"
echo "  orch-server: http://localhost:8080"
echo "  orch-agent:  http://localhost:8081"
echo "  metrics:     http://localhost:8080/metrics and http://localhost:8081/metrics"
