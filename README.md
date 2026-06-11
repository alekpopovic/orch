# orch

[![CI](https://github.com/alekpopovic/orch/actions/workflows/ci.yml/badge.svg)](https://github.com/alekpopovic/orch/actions/workflows/ci.yml)

`orch` is a Go container orchestrator for small, Docker-based clusters. It provides a control-plane API, a worker agent, and a CLI for deploying services, scaling replicas, assigning tasks to nodes, running Docker containers, checking health, streaming logs, and recording events.

The project is production-shaped but still an MVP. Package boundaries, tests, migrations, and interfaces are designed for production evolution; the default `orch-server` binary currently wires the in-memory control plane, so process restart recovery is not production-safe until the PostgreSQL-backed store is connected to the server runtime.

## What It Solves

- Deploy a named service from YAML or REST.
- Keep service replica count converged through task creation and stop directives.
- Assign pending tasks to ready nodes with deterministic scheduling.
- Run tasks as Docker containers through the Docker Engine API.
- Register agents, heartbeat nodes, and poll assigned tasks.
- Perform HTTP/TCP health checks and report healthy/unhealthy task state.
- Start rolling updates and rollbacks through asynchronous deployment objects.
- Stream task logs through the server from the owning agent.
- Emit event records through the active control plane implementation.
- Expose Prometheus metrics for the server and agent.

## What It Does Not Solve Yet

- The shipped `orch-server` process does not yet use PostgreSQL for its control-plane state.
- There is no HA leader election implementation; the reconciler has an abstraction only.
- Abrupt node loss is not marked offline by heartbeat expiry yet.
- Networking is Docker host/port based; there is no overlay network or service discovery.
- Secrets are represented as references, not a complete secret storage system.
- There is no multi-tenant isolation model.
- Agent authentication is token based; mTLS is documented as roadmap.
- Containerd support is a future runtime target.

## Binaries

- `orch-server`: REST API server and rollout controller.
- `orch-agent`: worker node process that talks to Docker and the server.
- `orch`: Cobra CLI for operators and scripts.

## Quickstart

Prerequisites:

- Go 1.25 or newer.
- Docker Engine and permission to access `/var/run/docker.sock`.
- Docker Compose v2.
- Bash, `curl`, and standard Unix shell tools.

Start PostgreSQL, `orch-server`, and one `orch-agent`:

```sh
./scripts/dev-up.sh
```

Apply migrations:

```sh
./scripts/migrate-up.sh
```

The migrations prepare PostgreSQL for the store package and integration tests. The current local `orch-server` still uses in-memory state.

Point the CLI at the server:

```sh
export ORCH_SERVER_URL=http://localhost:8080
go run ./cmd/orch node ls
```

Deploy the demo service:

```sh
./scripts/demo-deploy.sh
```

Operate it:

```sh
go run ./cmd/orch service ls
go run ./cmd/orch service ps http-api
go run ./cmd/orch scale http-api --replicas 2
go run ./cmd/orch rollout http-api --image nginx:1.28-alpine
go run ./cmd/orch events --service http-api
go run ./cmd/orch logs http-api --tail 100
go run ./cmd/orch delete http-api
```

Stop local services:

```sh
./scripts/dev-down.sh
```

## Build And Test

```sh
make build
make test
make lint
```

`make lint` requires `golangci-lint` on your `PATH`.

Useful direct commands:

```sh
go test ./...
go vet ./...
go build ./...
```

## CLI Configuration

The CLI resolves the server URL in this order:

1. `--server`
2. `ORCH_SERVER_URL`
3. `server_url` in `~/.config/orch/config.yaml`

Example:

```yaml
server_url: http://localhost:8080
```

Output formats:

```sh
orch service ls --output table
orch service inspect api --output json
```

Example deploy file:

```yaml
name: api
image: ghcr.io/example/api:1.0.0
replicas: 3
ports:
  - container: 8080
    public: 80
env:
  NODE_ENV: production
resources:
  cpu: 500m
  memory: 512Mi
healthcheck:
  type: http
  path: /health
  interval: 10s
  timeout: 2s
restart:
  policy: always
placement:
  labels:
    role: app
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [API](docs/API.md)
- [Agent](docs/AGENT.md)
- [Scheduler](docs/SCHEDULER.md)
- [Reconciler](docs/RECONCILER.md)
- [Rollouts](docs/ROLLOUTS.md)
- [Health checks](docs/HEALTHCHECKS.md)
- [Security](docs/SECURITY.md)
- [Observability](docs/OBSERVABILITY.md)
- [Reliability](docs/RELIABILITY.md)
- [Operations](docs/OPERATIONS.md)
- [Development](docs/DEVELOPMENT.md)
