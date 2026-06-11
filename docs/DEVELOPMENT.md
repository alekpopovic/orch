# Development

## Prerequisites

- Go 1.23 or newer
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

Integration tests that require Docker or PostgreSQL should document prerequisites and be separable from fast unit tests.

## Coding Notes

- Keep packages small and cohesive.
- Keep external systems behind interfaces.
- Make scheduler and reconciler logic deterministic.
- Ensure every long-running loop accepts context cancellation.
- Store and emit timestamps in UTC.
