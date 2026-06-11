# Observability

`orch-server` and `orch-agent` expose structured logs, events, and Prometheus metrics.

## Logs

Both binaries use structured logging through `log/slog`.

Important fields:

- request ID;
- method;
- route/path;
- status;
- duration;
- task ID, node ID, service ID, or rollout ID when relevant;
- error messages.

Do not log tokens, credentials, or secret values.

## Events

Events are operator-facing audit records. They include:

- type;
- severity: `info`, `warning`, or `error`;
- source;
- message;
- related object type and ID;
- timestamp.

Events are emitted for service changes, scheduler assignments, reconciler decisions, agent status changes, health failures, rollouts, and rollbacks.

Current implementation note: the default server stores events in memory. The PostgreSQL store can persist events, but server wiring to Postgres is not yet active.

## Metrics Endpoints

Server:

```text
GET /metrics
```

Agent:

```text
GET /metrics
```

In local Compose:

- server: `http://localhost:8080/metrics`
- agent: `http://localhost:8081/metrics`

## Server Metrics

- `api_requests_total{method,route,status}`
- `api_request_duration_seconds{method,route,status}`
- `scheduler_runs_total`
- `scheduler_errors_total`
- `scheduler_duration_seconds`
- `reconciler_runs_total`
- `reconciler_errors_total`
- `reconciler_duration_seconds`
- `tasks_created_total`
- `tasks_failed_total`
- `rollouts_total`
- `rollout_failures_total`

Current implementation note: API, task failure, and rollout request metrics are wired into the server binary. Scheduler and reconciler metric types exist and are used by their packages, but those loops are not all wired into the default `orch-server` process yet.

## Agent Metrics

- `heartbeat_success_total`
- `heartbeat_failure_total`
- `docker_operations_total{operation}`
- `docker_operation_errors_total{operation}`
- `task_state_changes_total{status}`
- `healthcheck_success_total`
- `healthcheck_failure_total`

Labels are intentionally low-cardinality. Do not add service names, task IDs, node IDs, image tags, or user-provided labels to Prometheus metrics.

## Prometheus Scrape Example

```yaml
scrape_configs:
  - job_name: orch-server
    static_configs:
      - targets: ["localhost:8080"]

  - job_name: orch-agent
    static_configs:
      - targets: ["localhost:8081"]
```

## Suggested Alerts

For an MVP environment:

- API 5xx rate above baseline.
- Agent heartbeat failures increasing.
- Docker operation errors increasing.
- Task failures increasing.
- Rollout failures non-zero.
- No events or metrics from an expected node.

Roadmap:

- Add service-level SLO metrics.
- Add node heartbeat expiry metrics.
- Add persistent audit export.
- Add trace spans around API/controller/store operations.
