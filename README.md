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

Start PostgreSQL plus optional local orchestrator services:

```sh
docker compose --profile local-orch up --build
```

## Binaries

- `orch`: operator CLI
- `orch-server`: control plane API server
- `orch-agent`: worker node agent

## Development

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for development commands and local workflow.
