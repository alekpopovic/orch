# Health Checks

Services may define an optional healthcheck. Health checks are performed by agents for running assigned tasks.

## Supported Types

- `http`: sends `GET` to the configured path and an assigned published TCP task port.
- `tcp`: opens a TCP connection to an assigned published TCP task port.
- `none`: disables active probing.

Example deploy YAML:

```yaml
healthcheck:
  type: http
  scheme: http
  path: /health
  interval: 10s
  timeout: 2s
  healthyThreshold: 2
  unhealthyThreshold: 3
```

## Agent Behavior

The server includes healthcheck metadata in task poll responses. For each running task:

1. The agent waits for the configured interval plus jitter.
2. It verifies that the healthcheck port matches one of the task's assigned published TCP container or host ports.
3. It performs the HTTP or TCP check against `<scheme>://127.0.0.1:<published-port><path>` with context cancellation and timeout.
4. If the service specified a container port, the agent resolves it to the assigned published host port before probing.
5. If the healthcheck port is not assigned, published, and TCP, the probe is skipped instead of probing an arbitrary host-local port.
6. It tracks consecutive successes and failures in memory.
7. It reports `healthy` after `healthyThreshold` consecutive successes.
8. It reports `unhealthy` after `unhealthyThreshold` consecutive failures.

The agent does not restart containers because of health failures.

HTTP healthchecks never accept arbitrary full URLs. `path` must begin with a single `/`; `scheme` may be `http` or `https` and defaults to `http`. The checker blocks redirects away from the originally constructed endpoint, including redirects to metadata services or external destinations, and reads at most 64 KiB of response body.

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
