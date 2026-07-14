# 11. Health checker

```text
Implement health checking for running tasks.

Support healthcheck types:
- HTTP
- TCP
- none

Service YAML example:
healthcheck:
  type: http
  path: /health
  interval: 10s
  timeout: 2s
  healthyThreshold: 2
  unhealthyThreshold: 3

Agent behavior:
- For each running assigned task, perform health checks.
- Report status to server:
  - healthy
  - unhealthy
- Do not restart directly unless the server/task directive says so.
- Respect context cancellation.
- Avoid synchronized checks by adding jitter.

Server/reconciler behavior:
- If a task is unhealthy beyond threshold, mark it failed or needs replacement depending on restart policy.
- Emit event.

Requirements:
- Add tests for HTTP and TCP health checks.
- Use httptest for HTTP tests.
- Add fake clock if helpful.
- Add docs/HEALTHCHECKS.md.
```
