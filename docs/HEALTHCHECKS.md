# Health Checks

Services may define an optional healthcheck. Supported types are:

- `http`: sends `GET` to the configured path.
- `tcp`: opens a TCP connection to the configured port.
- `none`: disables active probing.

Example deploy YAML:

```yaml
healthcheck:
  type: http
  path: /health
  interval: 10s
  timeout: 2s
  healthyThreshold: 2
  unhealthyThreshold: 3
```

## Agent Behavior

The control plane includes service healthcheck metadata when the agent polls assigned tasks. For each running assigned task, the agent performs the configured check with context cancellation and timeout support.

The agent keeps in-memory consecutive success and failure counters:

- after `healthyThreshold` consecutive successes, it reports `healthy`.
- after `unhealthyThreshold` consecutive failures, it reports `unhealthy`.

Checks include jitter derived from the service interval to avoid synchronized probing across tasks and nodes. The agent does not restart containers because of healthcheck failures; it only reports status to the control plane.

## Server Behavior

The control plane accepts `healthy` and `unhealthy` task status reports from agents. If a restartable service reports `unhealthy`, the control plane marks the task `failed` and emits a health failure event. The reconciler then creates replacement pending tasks according to service replica and restart policy rules.

For non-restartable services, unhealthy tasks remain `unhealthy` so operators can inspect them without automatic replacement.
