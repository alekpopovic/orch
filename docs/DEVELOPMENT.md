# Development

## Prerequisites

- Go 1.25 or newer
- Docker Engine
- Docker Compose
- PostgreSQL for integration work
- `golangci-lint` for linting

## Common Commands

```sh
go mod tidy
make build
make test
make lint
make run-server
make run-agent
```

Start local PostgreSQL:

```sh
docker compose up postgres
```

Start PostgreSQL plus the local orchestrator services:

```sh
cp .env.example .env
docker compose --profile local-orch up --build
```

## Testing

Fast unit tests must run with:

```sh
go test ./...
```

Integration tests that require PostgreSQL are skipped unless `ORCH_INTEGRATION_DATABASE_URL` is set:

```sh
docker compose up -d postgres
ORCH_INTEGRATION_DATABASE_URL="postgres://orch:orch@localhost:5432/orch?sslmode=disable" go test ./internal/store -run Integration
```

The store integration test reapplies the down and up migrations before running. Use a disposable local database.

## Migrations

Migrations live in `migrations/` as paired `*.up.sql` and `*.down.sql` files.

For local development with `psql`:

```sh
docker compose up -d postgres
psql "postgres://orch:orch@localhost:5432/orch?sslmode=disable" -f migrations/000001_initial_schema.up.sql
```

To roll back the initial schema:

```sh
psql "postgres://orch:orch@localhost:5432/orch?sslmode=disable" -f migrations/000001_initial_schema.down.sql
```

Apply migrations in order. Do not edit committed migrations; add a new migration instead.

## Coding Notes

- Keep packages small and cohesive.
- Keep external systems behind interfaces.
- Make scheduler and reconciler logic deterministic.
- Ensure every long-running loop accepts context cancellation.
- Store and emit timestamps in UTC.
