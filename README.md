# orch

[![CI](https://github.com/alekpopovic/orch/actions/workflows/ci.yml/badge.svg)](https://github.com/alekpopovic/orch/actions/workflows/ci.yml)

`orch` is a production-shaped Go container orchestrator scaffold. It targets multi-node Docker deployments through the Docker Engine API first, with package boundaries that can later support other runtimes.

## Quickstart

### Prerequisites

- Go 1.25 or newer.
- Docker Engine with access to `/var/run/docker.sock`.
- Docker Compose v2, available as `docker compose`.
- Bash, `curl`, and standard Unix shell tools.

No script requires root directly, but your user must be allowed to use Docker.

### Local Development Environment

Start PostgreSQL, `orch-server`, and one `orch-agent`:

```sh
./scripts/dev-up.sh
```

Apply the initial database schema:

```sh
./scripts/migrate-up.sh
```

Confirm the agent registered with the control plane:

```sh
export ORCH_SERVER_URL=http://localhost:8080
go run ./cmd/orch node ls
```

Deploy the demo app:

```sh
./scripts/demo-deploy.sh
```

Scale it:

```sh
go run ./cmd/orch scale http-api --replicas 2
```

View tasks, logs, and events:

```sh
go run ./cmd/orch service ps http-api
go run ./cmd/orch logs http-api --tail 100
go run ./cmd/orch events --service http-api
```

Delete the demo app:

```sh
go run ./cmd/orch delete http-api
```

Stop the local environment:

```sh
./scripts/dev-down.sh
```

Optional standalone demo container:

```sh
docker compose --profile demo up -d demo-app
curl http://localhost:18080
```

Build and test:

```sh
make build
make test
```

## Binaries

- `orch`: operator CLI
- `orch-server`: control plane API server
- `orch-agent`: worker node agent

## CLI Examples

The CLI reads the control-plane URL from `--server`, `ORCH_SERVER_URL`, or `server_url` in the CLI config file. By default the config file is `~/.config/orch/config.yaml`:

```yaml
server_url: http://localhost:8080
```

```sh
export ORCH_SERVER_URL=http://localhost:8080

orch version
orch node ls
orch node inspect 00000000-0000-4000-8000-000000000001
orch node drain 00000000-0000-4000-8000-000000000001
orch node uncordon 00000000-0000-4000-8000-000000000001
```

Deploy from YAML:

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

```sh
orch deploy service.yaml
orch service ls --output table
orch service inspect api --output json
orch service ps api
orch scale api --replicas 5
orch rollout api --image ghcr.io/example/api:1.0.1
orch rollout status api
orch rollback api
orch events
orch logs api --tail 100
orch logs api --follow
orch delete api
```

## Development

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for development commands and local workflow.
