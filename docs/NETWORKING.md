# Networking

The optional internal DNS MVP provides stable names over the host-port endpoint model; it does not create overlay addresses. Containers use the orch DNS listener and published service port. See `deployments/examples/internal-dns-compose.yaml` and [NETWORKING_DESIGN.md](NETWORKING_DESIGN.md).

`orch` supports host-port publishing for MVP services. It does not implement overlay networking yet.

## Service Ports

Service specs may declare ports with:

- `container_port`: container listener port;
- `published_port`: host port exposed on the selected node;
- `protocol`: `tcp` or `udp`.

If `published_port` is omitted or `0`, the scheduler assigns a dynamic host port from `30000-32767` on the selected node.

## Scheduling Rules

The scheduler rejects a node when an active task on that node already uses the requested `{protocol, published_port}` pair. The same host port may be used on different nodes.

During a single scheduling pass, planned assignments also reserve ports. This prevents two pending tasks from receiving the same dynamic port on the same node.

When Docker reports a port collision, the agent marks the task `failed` with a failure reason beginning with `port allocation failed:`. The scheduler treats that failed task as a temporary node-local port reservation so the reconciler-created replacement can move to another node when one is available.

## Agent Runtime

Assigned task ports are stored on task records and included in the agent task payload. The agent passes those assigned ports to the container runtime when creating containers.

## Limitations

- No overlay network.
- No service VIP or DNS in this prompt.
- Dynamic port allocation is deterministic per scheduling pass, not a cluster-wide lease service.
