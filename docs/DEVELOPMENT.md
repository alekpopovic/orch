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

Docker runtime integration tests are skipped unless `ORCH_DOCKER_INTEGRATION=1` is set:

```sh
ORCH_DOCKER_INTEGRATION=1 go test ./internal/docker -run Integration
```

These tests talk to the Docker daemon configured by the standard Docker environment variables such as `DOCKER_HOST`, `DOCKER_TLS_VERIFY`, and `DOCKER_CERT_PATH`.

## Docker Daemon Access

The worker agent needs permission to call the Docker Engine API for image pulls, container create/start/stop/remove, inspect, list, and log streaming. In local development this usually means access to `/var/run/docker.sock` or a Docker context configured through environment variables.

Docker daemon access is highly privileged. A process that can write to the Docker socket can usually start containers with host mounts, read host files, access container secrets, and affect other workloads on the node. Production deployments should:

- Run `orch-agent` only on trusted worker nodes.
- Scope Docker socket or TCP API access to the agent process.
- Prefer TLS-protected Docker API endpoints when using TCP.
- Avoid mounting the Docker socket into untrusted containers.
- Treat registry credentials passed to image pull as secrets and never log them.
- Rely on orchestrator labels such as `orch.managed=true` when listing or cleaning up containers.

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
