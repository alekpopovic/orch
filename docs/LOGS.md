# Logs

The MVP logs path streams container logs from worker agents through the control-plane API.

```sh
orch logs api
orch logs api --tail 100
orch logs api --follow
orch logs api --task 00000000-0000-4000-8000-000000000123
```

## Flow

1. The CLI resolves the service name or ID and calls `GET /v1/logs`.
2. The server selects a running task for the service, or uses the explicit `task_id`.
3. The server checks that the task has a container and that its node is not offline.
4. The server proxies the request to the node agent at `GET /v1/agent/logs`.
5. The agent streams Docker logs for the managed container with the matching `orch.task_id` label.

The response is newline-delimited text over chunked HTTP. `--follow` keeps the stream open until the client disconnects or the request context is canceled.

## Limitations

- Logs are proxied from one task at a time.
- Service-level logs pick the first running or healthy task with a container.
- There is no persisted log store yet.
- If the node is offline or unreachable, logs are unavailable.
- The control plane depends on each node's `advertise_address` pointing to the agent HTTP listener.

Agents protect the log endpoint with the bootstrap token for now. mTLS should replace this before production multi-tenant use.
