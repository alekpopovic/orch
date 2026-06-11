# Health Checks

Services may define an optional healthcheck. Health checks are performed by agents for running assigned tasks.

## Supported Types

- `http`: sends `GET` to the configured path and port.
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

The server includes healthcheck metadata in task poll responses. For each running task:

1. The agent waits for the configured interval plus jitter.
2. It performs the HTTP or TCP check with context cancellation and timeout.
3. It tracks consecutive successes and failures in memory.
4. It reports `healthy` after `healthyThreshold` consecutive successes.
5. It reports `unhealthy` after `unhealthyThreshold` consecutive failures.

The agent does not restart containers because of health failures.

## Server Behavior

The server accepts `healthy` and `unhealthy` task status reports.

For restartable services, an `unhealthy` report is converted to task `failed`; the event stream records a health failure and the reconciler can create a replacement task.

For non-restartable services, the task remains `unhealthy` so an operator can inspect it.

## Rollout Interaction

Rollouts wait for target-version tasks to become `healthy`. A target-version `failed` task marks the rollout `failed`.

Services without a healthcheck rely on task runtime status. In that case, rollout health gating is limited by the statuses the agent reports.

## Current Limitations

- Health counters are in agent memory; agent restart resets consecutive counts.
- No global health history store yet.
- No gRPC or command healthcheck type.
- No service-level readiness aggregation endpoint.
