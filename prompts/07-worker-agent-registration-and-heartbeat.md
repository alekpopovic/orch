# 07. Worker agent registration and heartbeat

```text
Implement the worker agent registration and heartbeat flow.

Agent behavior:
- Start with config:
  - server URL
  - node name
  - advertise address
  - labels
  - heartbeat interval
  - Docker socket path
- Detect local node capacity:
  - CPU cores
  - memory
- Register node with control plane.
- Send heartbeat every N seconds.
- Mark itself draining/offline only based on server response or shutdown lifecycle.
- On graceful shutdown, notify server if possible.

Server behavior:
- Add internal endpoints for agents:
  - POST /v1/agent/register
  - POST /v1/agent/heartbeat
- Store node information.
- Update last heartbeat.
- Return node status and any control-plane directives.

Requirements:
- Use context cancellation.
- Add retry with exponential backoff.
- Add jitter.
- Avoid thundering herd.
- Add tests for heartbeat state transitions.
- Add docs/AGENT.md explaining agent lifecycle.

Security:
- For now use a static bootstrap token from config.
- Structure the code so mTLS can replace this later.
```
