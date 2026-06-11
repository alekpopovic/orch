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
- `ORCH_AGENT_REGISTRATION_TOKEN`: static registration token used to obtain a short-lived agent credential until stronger node identity is added.

The agent detects local capacity from the host:

- CPU as millicores from `runtime.NumCPU()`.
- Memory from `/proc/meminfo` when available.

It then calls `POST /v1/agent/register`. The control plane creates or updates the node record and returns the authoritative node status plus any directives.

## Heartbeats

After registration, the agent sends `POST /v1/agent/heartbeat` every configured interval. The loop uses context cancellation, exponential backoff, and jitter so reconnecting agents do not all retry at the same instant.

The agent does not choose to become draining on its own. Draining is controlled by the server response, usually after an operator calls `orch node drain`. The agent records and logs the returned status and directives.

## Task Assignment

After each successful heartbeat, the agent calls `GET /v1/agent/tasks?node_id=<id>`. The control plane remains the source of truth and returns the desired tasks currently assigned to that node.

For each assigned task with desired status `running`, the agent reconciles Docker state idempotently:

- report `pulling`, pull the image, and retry safely if the pull fails.
- create a managed container labeled with service, task, node, and version identity.
- report `created` once a container ID exists.
- start the container and report `running`.

The agent may also report `unhealthy`, `failed`, `stopped`, or `removed` through `POST /v1/agent/tasks/{task_id}/status`. Status reports include the node ID so the server can reject updates for tasks assigned elsewhere.

On every poll, the agent lists locally managed Docker containers for its node. Containers whose `orch.task_id` is no longer assigned are stopped and removed. This makes process restarts safe without a local durable cache: Docker labels provide local identity, and the server provides desired state.

## Health Checks

For running assigned tasks with a service healthcheck, the agent performs HTTP or TCP probes and reports `healthy` or `unhealthy` only after the configured consecutive threshold is met. Probes respect context cancellation, use the service timeout, and add jitter to avoid synchronized checks. See `docs/HEALTHCHECKS.md`.

## Shutdown

On graceful shutdown, the agent sends a final heartbeat with `shutdown=true`. If the notification succeeds, the control plane marks the node offline. If it fails, the agent logs a warning and continues shutdown.

## Security

Registration uses a static registration token:

```text
Authorization: Bearer <ORCH_AGENT_REGISTRATION_TOKEN>
```

After registration, the server returns a short-lived agent credential. The agent uses that credential for heartbeat, task polling, and task status updates. Heartbeat responses rotate the credential.

This is intentionally isolated to the agent endpoint boundary so mTLS-based node identity can replace or combine with token credentials later. Treat tokens as secrets. Do not commit them, print them, or expose them through logs.

Access to the Docker socket is highly privileged. A compromised agent can control containers on the node and may be able to affect the host. See `docs/DEVELOPMENT.md` for Docker daemon permission risks.
