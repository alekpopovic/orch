# Agent

`orch-agent` is the worker-node process. It is responsible for local Docker reconciliation only; the server remains the source of truth for desired tasks.

## Configuration

Environment variables:

- `ORCH_SERVER_URL`: control-plane URL. Default: `http://localhost:8080`.
- `ORCH_NODE_NAME`: stable node name. Default: `local-node`.
- `ORCH_ADVERTISE_ADDRESS`: address operators/server use to reach the agent log endpoint. Default: `http://127.0.0.1:8081`.
- `ORCH_AGENT_ADDR`: listen address for agent logs/metrics. Default: `:8081`.
- `ORCH_NODE_LABELS`: comma-separated labels such as `role=worker,zone=a`.
- `ORCH_AGENT_HEARTBEAT_INTERVAL`: heartbeat interval. Default: `5s`.
- `ORCH_DOCKER_SOCKET`: Docker socket path. Default: `/var/run/docker.sock`.
- `ORCH_AGENT_REGISTRATION_TOKEN`: registration token.
- `ORCH_LOG_LEVEL`: structured log level. Default: `info`.
- `ORCH_SHUTDOWN_TIMEOUT`: graceful shutdown timeout. Default: `10s`.

## Startup

On startup the agent:

1. Loads config.
2. Creates a Docker Engine runtime client.
3. Detects CPU capacity from `runtime.NumCPU()`.
4. Detects memory from `/proc/meminfo` when available.
5. Calls `POST /v1/agent/register` with the registration token.
6. Receives a node ID, node status, directives, and short-lived credential.
7. Starts the heartbeat and task reconciliation loop.

Registration is keyed by stable node name in the current in-memory control plane. If a node returns after graceful offline, registering with the same name reuses the node ID.

## Heartbeats

The agent sends `POST /v1/agent/heartbeat` every interval. The loop supports context cancellation, exponential backoff, and jitter.

Heartbeats update capacity, allocatable resources, labels, and last heartbeat time. A graceful shutdown heartbeat sets `shutdown=true`; the server marks the node `offline` if it receives it.

Roadmap: a server-side heartbeat expiry controller should mark abruptly lost nodes offline.

## Task Polling

After successful heartbeats, the agent polls:

```text
GET /v1/agent/tasks?node_id=<node-id>
```

The response contains assigned tasks plus service healthcheck and port metadata.

## Docker Reconciliation

For each assigned task whose desired status is `running`, the agent:

1. Looks for an existing task container by container ID.
2. If needed, searches Docker by orchestrator labels:
   - `orch.managed=true`
   - `orch.service_id`
   - `orch.task_id`
   - `orch.node_id`
   - `orch.version`
3. If a running container is found, reports `running` with the recovered container ID.
4. If a stopped container exited non-zero, reports `failed`.
5. If no container exists and Docker is reachable, pulls the image, creates the container, starts it, and reports status after each step.

The agent does not create a replacement if Docker cannot list managed containers. That avoids duplicates during Docker daemon restarts or transient Docker API failures.

## Stop And Cleanup

For tasks whose desired status is `stopped` or `removed`, the agent stops and removes the managed container when possible, then reports terminal status.

On every poll, the agent lists local managed containers for its node. Containers with `orch.managed=true` and no corresponding assigned task are stopped and removed. Cleanup is limited to orchestrator-managed containers for the current node.

## Health Checks

The agent performs HTTP/TCP checks for running tasks with a configured healthcheck. It reports `healthy` or `unhealthy` only after the configured consecutive threshold is met. It does not restart containers based on health; restart/replacement is a control-plane decision.

## Logs And Metrics

The agent exposes:

- `GET /metrics`
- log streaming endpoints used by the server log proxy

The log endpoint reads from Docker and streams without loading unbounded logs into memory.

## Security Considerations

The Docker socket is highly privileged. A compromised agent can usually control containers on the node and may affect the host. Only run trusted agent binaries, protect the registration token, and restrict network access to the agent log endpoint.
