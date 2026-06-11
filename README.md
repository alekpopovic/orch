# orch

`orch` is a production-shaped Go container orchestrator scaffold. It targets multi-node Docker deployments through the Docker Engine API first, with package boundaries that can later support other runtimes.

## Quickstart

Build and test:

```sh
make build
make test
```

Run the control plane API server:

```sh
cp .env.example .env
make run-server
```

In another shell, run an agent:

```sh
make run-agent
```

Start local PostgreSQL:

```sh
docker compose up postgres
```

Apply the initial schema:

```sh
psql "postgres://orch:orch@localhost:5432/orch?sslmode=disable" -f migrations/000001_initial_schema.up.sql
```

Start PostgreSQL plus optional local orchestrator services:

```sh
docker compose --profile local-orch up --build
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
