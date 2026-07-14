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

Events are operator-facing orchestration history records. They include:

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
- `scheduler_scheduling_attempts_total`
- `scheduler_scheduling_failures_total`
- `scheduler_tasks_claimed_total`
- `scheduler_assignment_conflicts_total`
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
rule_files:
  - /etc/prometheus/rules/prometheus-rules.yaml

scrape_configs:
  - job_name: orch-server
    static_configs:
      - targets: ["localhost:8080"]

  - job_name: orch-agent
    static_configs:
      - targets: ["localhost:8081"]
```

Local Compose can run Prometheus with the bundled rules:

```sh
docker compose --profile local-orch --profile monitoring up -d
open http://localhost:9090
```

The local monitoring profile starts Prometheus and a PostgreSQL exporter. It loads `deploy/monitoring/prometheus.yml` and `deploy/monitoring/prometheus-rules.yaml`.

## Recommended Alerts

Recommended alert rules live in `deploy/monitoring/prometheus-rules.yaml`.

| Alert | Severity | Meaning | First response |
| --- | --- | --- | --- |
| `OrchServerDown` | critical | Prometheus cannot scrape `orch-server`. | Check server process, container status, and port `8080`. |
| `OrchAgentHeartbeatMissing` | warning | An agent stopped recording successful heartbeats. | Check agent logs, server reachability, token rotation, and clock skew. |
| `NodeOffline` | critical | Prometheus cannot scrape an agent endpoint. | Treat the node as suspect; inspect `orch node inspect` and Docker health. |
| `SchedulerErrorsHigh` | warning | Scheduler errors are increasing. | Inspect scheduler logs and pending task placement constraints. |
| `ReconcilerErrorsHigh` | warning | Reconciler errors are increasing. | Inspect service/task state and control-plane logs. |
| `RolloutFailed` | critical | A rollout entered failed state. | Run `orch rollout status`, `orch events --service`, and inspect failed tasks. |
| `TaskFailuresHigh` | warning | Task failures exceeded the recommended baseline. | Check image pulls, healthchecks, port collisions, and node runtime errors. |
| `DockerOperationErrorsHigh` | warning | Docker API operations are failing across agents. | Check Docker daemon health and socket permissions on affected nodes. |
| `DatabaseUnavailable` | critical | PostgreSQL exporter cannot be scraped. | Check PostgreSQL, exporter credentials, network path, and migrations. |
| `APIErrorRateHigh` | warning | More than 5% of API requests are 5xx responses. | Correlate API logs by request ID and inspect recent deploy/rollout activity. |
| `ReconciliationLatencyHigh` | warning | Reconciler p95 runtime is above 10 seconds. | Check database latency, event backlog, and controller contention. |

Alert labels intentionally stay low-cardinality: `severity` and `component`. Do not add service IDs, task IDs, user IDs, image names, or node labels to alert labels; put those in annotations or linked runbooks.

Roadmap:

- Add service-level SLO metrics.
- Add node heartbeat expiry metrics.
- Add persistent audit export.
- Add trace spans around API/controller/store operations.
