# Observability

`orch-server` and `orch-agent` expose Prometheus metrics at:

- `GET /metrics` on the server address, for example `http://localhost:8080/metrics`.
- `GET /metrics` on the agent address, for example `http://localhost:8081/metrics`.

## Server Metrics

- `api_requests_total{method,route,status}`: API requests by HTTP method, route template, and status code.
- `api_request_duration_seconds{method,route,status}`: API request latency.
- `scheduler_runs_total`: scheduler pass count.
- `scheduler_errors_total`: scheduler pass failures.
- `scheduler_duration_seconds`: scheduler pass duration.
- `reconciler_runs_total`: reconciler pass count.
- `reconciler_errors_total`: reconciler pass failures.
- `reconciler_duration_seconds`: reconciler pass duration.
- `tasks_created_total`: tasks created by controllers.
- `tasks_failed_total`: tasks reported failed.
- `rollouts_total`: rollout or rollback requests accepted.
- `rollout_failures_total`: rollouts marked failed.

API metrics use route templates such as `/v1/services/{id}` rather than raw paths. This keeps labels low-cardinality and avoids embedding service IDs, task IDs, node IDs, or names in Prometheus series.

## Agent Metrics

- `heartbeat_success_total`: successful heartbeat attempts.
- `heartbeat_failure_total`: failed heartbeat attempts.
- `docker_operations_total{operation}`: Docker runtime operations by operation type.
- `docker_operation_errors_total{operation}`: Docker runtime operation failures by operation type.
- `task_state_changes_total{status}`: task status updates accepted by the control plane.
- `healthcheck_success_total`: successful health checks.
- `healthcheck_failure_total`: failed health checks.

Agent labels are intentionally limited to operation and status enums. Do not add labels for service names, task IDs, container IDs, node IDs, image tags, or user-provided labels unless there is a reviewed operational reason.

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

For production, discover agents through your infrastructure inventory or service discovery rather than hard-coding every node.
