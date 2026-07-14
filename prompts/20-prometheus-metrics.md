# 20. Prometheus metrics

```text
Implement Prometheus metrics.

Expose:
- /metrics on orch-server
- /metrics on orch-agent

Server metrics:
- api_requests_total
- api_request_duration_seconds
- scheduler_runs_total
- scheduler_errors_total
- scheduler_duration_seconds
- reconciler_runs_total
- reconciler_errors_total
- reconciler_duration_seconds
- tasks_created_total
- tasks_failed_total
- rollouts_total
- rollout_failures_total

Agent metrics:
- heartbeat_success_total
- heartbeat_failure_total
- docker_operations_total
- docker_operation_errors_total
- task_state_changes_total
- healthcheck_success_total
- healthcheck_failure_total

Requirements:
- Use prometheus/client_golang.
- Keep metric labels low-cardinality.
- Do not use service name or task ID as high-cardinality labels unless justified.
- Add docs/OBSERVABILITY.md.
- Add tests where practical.
```
