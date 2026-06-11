# Agent Lifecycle

`orch-agent` is the worker process that represents one node to the control plane.

## Startup

On startup the agent loads:

- `ORCH_SERVER_URL`: control-plane API URL.
- `ORCH_NODE_NAME`: stable node name.
- `ORCH_ADVERTISE_ADDRESS`: address the control plane and operators should use for the node.
- `ORCH_NODE_LABELS`: comma-separated labels such as `role=app,zone=a`.
- `ORCH_AGENT_HEARTBEAT_INTERVAL`: heartbeat period.
- `ORCH_DOCKER_SOCKET`: Docker Engine socket path.
- `ORCH_BOOTSTRAP_TOKEN`: static bootstrap token used until stronger node identity is added.

The agent detects local capacity from the host:

- CPU as millicores from `runtime.NumCPU()`.
- Memory from `/proc/meminfo` when available.

It then calls `POST /v1/agent/register`. The control plane creates or updates the node record and returns the authoritative node status plus any directives.

## Heartbeats

After registration, the agent sends `POST /v1/agent/heartbeat` every configured interval. The loop uses context cancellation, exponential backoff, and jitter so reconnecting agents do not all retry at the same instant.

The agent does not choose to become draining on its own. Draining is controlled by the server response, usually after an operator calls `orch node drain`. The agent records and logs the returned status and directives.

## Shutdown

On graceful shutdown, the agent sends a final heartbeat with `shutdown=true`. If the notification succeeds, the control plane marks the node offline. If it fails, the agent logs a warning and continues shutdown.

## Security

The first version uses a static bootstrap token:

```text
Authorization: Bearer <ORCH_BOOTSTRAP_TOKEN>
```

This is intentionally isolated to the agent endpoint boundary so mTLS-based node identity can replace it later. Treat the token as a secret. Do not commit it, print it, or expose it through logs.

Access to the Docker socket is highly privileged. A compromised agent can control containers on the node and may be able to affect the host. See `docs/DEVELOPMENT.md` for Docker daemon permission risks.
