#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export ORCH_SERVER_URL="${ORCH_SERVER_URL:-http://localhost:8080}"

echo "Waiting for orch-server at ${ORCH_SERVER_URL}..."
until curl -fsS "${ORCH_SERVER_URL}/readyz" >/dev/null; do
  sleep 1
done

if go run ./cmd/orch service inspect http-api >/dev/null 2>&1; then
  echo "Demo HTTP API already exists."
else
  echo "Deploying demo HTTP API..."
  go run ./cmd/orch deploy deployments/examples/http-api.yaml
fi

echo
echo "Current services:"
go run ./cmd/orch service ls

echo
echo "Current nodes:"
go run ./cmd/orch node ls
