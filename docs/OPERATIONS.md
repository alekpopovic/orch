# Operations

This guide describes how to run and operate the current MVP safely.

## Deployment Modes

### Local Development

Use Docker Compose:

```sh
./scripts/dev-up.sh
./scripts/migrate-up.sh
export ORCH_SERVER_URL=http://localhost:8080
go run ./cmd/orch node ls
```

Stop:

```sh
./scripts/dev-down.sh
```

### Production Assumption

The codebase is not production-ready as a durable orchestrator until `orch-server` is wired to PostgreSQL. The current server process stores control-plane state in memory.

Production deployment examples are documented in [PRODUCTION_DEPLOYMENT.md](https://alekpopovic.github.io/orch/#PRODUCTION_DEPLOYMENT.md).

Before production use:

- connect `orch-server` to `PostgresStore`;
- run migrations at deploy time;
- configure JWT auth;
- protect agent registration token or replace with mTLS;
- run agents only on trusted nodes with Docker access;
- add real leader election before multiple controllers;
- define backup and restore procedures for PostgreSQL.

## Configuration

Full precedence, YAML file examples, and redacted config output are documented in [CONFIGURATION.md](https://alekpopovic.github.io/orch/#CONFIGURATION.md).

Server:

- `ORCH_SERVER_ADDR`: listen address, default `:8080`.
- `DATABASE_URL`: PostgreSQL URL, loaded but not used by the current server wiring.
- `ORCH_AGENT_REGISTRATION_TOKEN`: agent bootstrap token.
- `ORCH_JWT_SECRET`: enables JWT auth when set.
- `ORCH_USERS`: optional static user role map.
- `ORCH_LOG_LEVEL`: log level.
- `ORCH_SHUTDOWN_TIMEOUT`: graceful shutdown timeout.
- `ORCH_NODE_HEARTBEAT_TIMEOUT`: stale heartbeat timeout before a node is marked offline, default `30s`.
- `ORCH_NODE_MONITOR_INTERVAL`: node monitor check interval, default `5s`.

Agent:

- `ORCH_SERVER_URL`
- `ORCH_NODE_NAME`
- `ORCH_ADVERTISE_ADDRESS`
- `ORCH_AGENT_ADDR`
- `ORCH_NODE_LABELS`
- `ORCH_AGENT_REGISTRATION_TOKEN`
- `ORCH_DOCKER_SOCKET`
- `ORCH_AGENT_HEARTBEAT_INTERVAL`
- `ORCH_LOG_LEVEL`
- `ORCH_SHUTDOWN_TIMEOUT`

## Migrations

Apply:

```sh
./scripts/migrate-up.sh
```

Rollback:

```sh
./scripts/migrate-down.sh
```

Migration files live in `migrations/`.

The production-safe runner exposes `orch-server migrate status|up|down`; `down` additionally requires `--allow-down`. See [MIGRATIONS.md](https://alekpopovic.github.io/orch/#MIGRATIONS.md).

## Maintenance Windows

Create namespace-scoped or global windows with a five-field cron schedule, IANA timezone, duration, and allowed operations:

```sh
orch -n payments maintenance create weekly \
  --schedule "0 2 * * 0" --timezone Europe/Belgrade --duration 2h \
  --operations rollout,rollback,node_drain,autoscaling_scale_down
orch -n payments maintenance ls
orch -n payments maintenance delete <window-id>
```

Applicable rollouts, rollbacks, drains, and scale-downs are rejected outside an enabled window. An emergency operator may add `--force`; the bypass is recorded in the audit log. Critical safety replacement is not delayed by default.

## Retention and Usage

The server captures namespace usage every minute and runs the retention pruner daily. Inspect or preview it before manual pruning:

```sh
orch retention status
orch retention prune --dry-run
orch retention prune
orch usage --namespace payments --from 2026-08-01 --to 2026-09-01
orch usage export --namespace payments --from 2026-08-01 --to 2026-09-01 --format csv
```

Retention never removes active services, running tasks, active rollouts, or unresolved failures. Completed pruning emits an audit record. Usage reports accounting measurements only; they do not implement billing, payments, or invoices.

## Runbooks

### Check Cluster Health

```sh
curl http://localhost:8080/readyz
orch node ls
orch service ls
orch events --output table
```

Check metrics:

```sh
curl http://localhost:8080/metrics
curl http://localhost:8081/metrics
```

### Deploy A Service

```sh
orch deploy deployments/examples/http-api.yaml
orch service inspect http-api
orch service ps http-api
```

### Scale A Service

```sh
orch scale http-api --replicas 3
orch service ps http-api
orch events --service http-api
```

Expected behavior: the reconciler/control plane creates or stops tasks; scheduler assigns new pending tasks; agents converge Docker containers.

### Roll Out A New Image

```sh
orch rollout http-api --image nginx:1.28-alpine
orch rollout status http-api
orch service ps http-api
```

If rollout fails, inspect:

```sh
orch events --service http-api
orch service ps http-api --output json
orch logs http-api --tail 100
```

### Roll Back

```sh
orch rollback http-api
orch rollout status http-api
```

Rollback requires a previous successful version.

### Drain A Node

```sh
orch node drain <node-id>
orch node drain-status <node-id>
```

Draining marks the node `draining`, excludes it from new placements, and creates replacement tasks on other ready nodes. Existing tasks on the drained node are stopped only after replacements become `running` or `healthy`.

If there is no ready replacement capacity, drain remains pending and emits a warning event:

```sh
orch events --type node.drain.pending
orch node drain-status <node-id> --output json
```

If a node goes offline during drain, inspect status and events before force-removing workloads:

```sh
orch node inspect <node-id>
orch events --type node.status.changed
```

Uncordon:

```sh
orch node uncordon <node-id>
```

Uncordon returns the node to `ready` and allows new placements again. If replacements were already created, normal reconciliation may later remove excess tasks.

### Investigate Task Failure

```sh
orch service ps <service>
orch events --service <service>
orch logs <service> --task <task-id> --tail 200
```

Common causes:

- image pull failure;
- port already allocated;
- container exited non-zero;
- healthcheck threshold exceeded;
- Docker daemon unavailable.

### Review Audit Logs

Audit logs are separate from operator events and capture who attempted mutating actions, what target was affected, request IDs, source IPs, outcomes, and redacted metadata.

```sh
orch audit
orch audit --actor-type user --actor-id alice
orch audit --action service.rollout --outcome failure
orch audit --target-type service --target-id <service-id> --output json
```

Use audit request IDs to correlate with API access logs. Secret plaintext, registry passwords, tokens, and credential material should appear only as `[REDACTED]`.

### Delete A Service

```sh
orch delete <service>
orch service inspect <service>
orch service ps <service>
```

Delete is soft. The service remains `deleting` until agents report task containers removed.

### Agent Cannot Register

Check:

- `ORCH_SERVER_URL`;
- `ORCH_AGENT_REGISTRATION_TOKEN` on both server and agent;
- server logs;
- network reachability from agent to server;
- timeouts and DNS in Docker Compose.

### Logs Unavailable

Logs are proxied from the owning agent. Check:

- task has a container ID;
- node is not offline;
- agent advertise address is reachable from server;
- Docker daemon is reachable on the node;
- agent log endpoint is running.

## Backups

Detailed backup and restore procedures are documented in [BACKUP_RESTORE.md](https://alekpopovic.github.io/orch/#BACKUP_RESTORE.md).

For future PostgreSQL-backed production, back up:

- `nodes`;
- `services`;
- `service_versions`;
- `tasks`;
- `deployments`;
- `events`.
- `audit_logs`.

Until server state is wired to PostgreSQL, backups do not preserve live orchestrator state.

## Upgrade Notes

- Drain or stop agents before changing Docker socket permissions.
- Roll server and agents separately.
- Avoid running multiple scheduler/reconciler instances without leader election.
- Run migrations before deploying binaries that depend on schema changes.

## Controller Leadership

HA deployments should run API servers active-active, but background controllers must be active-passive per controller name. Use the `internal/leadership` PostgreSQL advisory-lock elector when wiring production controllers.

Recommended lock names:

- `scheduler`
- `reconciler`
- `rollout`
- `node-monitor`
- `autoscaler`

Monitor `controller_leader_status` and `controller_leader_acquisition_failures_total` to confirm exactly one controller leader is active and to detect lock contention or database connectivity issues.
