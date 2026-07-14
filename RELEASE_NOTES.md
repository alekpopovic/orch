# Release Notes

## v0.1.0 MVP

This is the first MVP release candidate for `orch`, a Go-based Docker container orchestrator.

### What Works

- `orch-server` starts with structured logs, health endpoints, optional JWT auth, agent auth, request IDs, JSON errors, and Prometheus metrics.
- `orch-agent` registers with the server, heartbeats, receives short-lived credentials, polls assigned tasks, and reconciles Docker containers.
- `orch` CLI can deploy services from YAML, list/inspect services and nodes, scale services, view tasks, stream logs, list events, start rollouts, request rollback, and delete services.
- Services model image, replicas, env, ports, resource requests/limits, healthchecks, restart policy, placement constraints, and deployment version.
- Task lifecycle supports pending, assigned, pulling, created, running, healthy, unhealthy, failed, stopped, and removed states.
- Scheduler and reconciler packages are deterministic and unit-tested.
- Agent Docker execution is idempotent by task/container labels and handles process restart, missing containers, non-zero exits, pull failures, and create/start errors.
- Soft service deletion marks services deleting, stops/removes task containers, and finalizes after tasks report removed.
- Logs are proxied from the owning agent without unbounded buffering.
- Events are queryable through the API and CLI for the active control-plane implementation.
- PostgreSQL migrations and store implementation exist for nodes, services, service versions, tasks, deployments, and events.
- Local Docker Compose environment starts PostgreSQL, server, and one agent.

### Important MVP Limits

- The default `orch-server` binary still uses the in-memory control plane. PostgreSQL-backed server state is not wired yet.
- Server process restart loses in-memory services, tasks, deployments, events, node credentials, and node status.
- No heartbeat-expiry controller marks abruptly lost nodes offline yet.
- No HA leader election implementation yet.
- Networking is Docker host/port based; no overlay networking or service discovery.
- Agent authentication is token based; mTLS is roadmap.
- Secrets are represented as references only.
- Rollouts do not yet support manual pause/resume, progress deadlines, or automatic rollback.

### Verification For This Candidate

- Full Go test suite: `go test ./...`
- Build: `go build ./...`
- Vet: `go vet ./...`
- Lint target: `make lint`
- MVP fake-runtime E2E covers agent registration, service deploy, task creation/assignment, task running, log streaming, events, scale up/down, and delete cleanup.

### Local Demo

```sh
./scripts/dev-up.sh
./scripts/migrate-up.sh
export ORCH_SERVER_URL=http://localhost:8080
./scripts/demo-deploy.sh
go run ./cmd/orch service ps http-api
go run ./cmd/orch logs http-api --tail 100
go run ./cmd/orch events --service http-api
go run ./cmd/orch scale http-api --replicas 2
go run ./cmd/orch delete http-api
./scripts/dev-down.sh
```

### Upgrade Notes

This is the first release; no upgrade path is required. For future releases, run migrations before deploying binaries that depend on schema changes.

## v0.2.0 Production Hardening

`orch` v0.2.0 focuses on production hardening around observability, security, backup/restore, deployment manifests, autoscaling foundations, HA controller coordination, and reliability testing.

### Highlights

- Added Prometheus alert rules and a Grafana dashboard.
- Added backup and restore scripts plus operator runbooks.
- Added production Docker Compose and systemd deployment examples.
- Completed a security review and fixed the high-risk healthcheck host-local SSRF issue.
- Added default container security policy enforcement for privileged mode, host network/PID, host paths, and Linux capabilities.
- Hardened healthcheck validation and execution with scheme/path-only specs, redirect blocking, assigned-port enforcement, and response body read limits.
- Added autoscaling design and CPU-based autoscaler MVP using a metrics provider abstraction.
- Added HA control-plane design and PostgreSQL advisory-lock leader election primitives.
- Added multi-agent load testing and chaos-style integration scenarios.

### Compatibility Notes

- New migrations add `security_context` and `autoscaling` JSONB fields to `services`.
- Existing service specs remain compatible because new fields default to empty safe values.
- The default security policy rejects privilege-escalating options unless explicitly allowlisted.
- The autoscaler controller uses the existing service scale path and skips active rollouts.

### Verification

Completed for this candidate:

- `go test ./...`
- `go build ./...`
- `docker compose config`
- security-sensitive grep checks for token/secret logging

`golangci-lint` was not installed in the local environment, so lint could not be executed locally.

### Known Limits

- The default server still uses the in-memory control plane.
- PostgreSQL store and migration support exist, but full server runtime wiring to durable state remains future work.
- Autoscaling includes the controller and fake metrics provider; a production Prometheus provider is not wired yet.
- HA leader election primitives are implemented, but production controller wiring still needs DB-backed server state.
- GitHub reported existing Dependabot vulnerabilities on push; review the repository security tab before tagging.

## v0.1.0 MVP

The first MVP release candidate for `orch`, a Go-based Docker container orchestrator.

### What Works

- `orch-server` starts with structured logs, health endpoints, optional JWT auth, agent auth, request IDs, JSON errors, and Prometheus metrics.
- `orch-agent` registers with the server, heartbeats, receives short-lived credentials, polls assigned tasks, and reconciles Docker containers.
- `orch` CLI can deploy services from YAML, list/inspect services and nodes, scale services, view tasks, stream logs, list events, start rollouts, request rollback, and delete services.
- Services model image, replicas, env, ports, resource requests/limits, healthchecks, restart policy, placement constraints, and deployment version.
- Scheduler and reconciler packages are deterministic and unit-tested.
- Soft service deletion marks services deleting, stops/removes task containers, and finalizes after tasks report removed.

### Upgrade Notes

This is the first release series. For future releases, run migrations before deploying binaries that depend on schema changes.
