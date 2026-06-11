# Events

Events are orchestration audit records emitted by the API, scheduler, reconciler, agent status path, health handling, and rollout controller.

Current implementation note: the default `orch-server` stores events in memory because it uses the in-memory control plane. The PostgreSQL store can persist events once the server is wired to it.

## Sources

Events are emitted by:

- service create, scale, rollout, rollback, and delete operations;
- scheduler task assignments;
- reconciler task creation and stop decisions;
- agent task status reports;
- healthcheck unhealthy/failure transitions;
- rollout status changes and task decisions;
- node registration, heartbeat, shutdown, and status changes.

Event emission is best-effort for most controller paths. A failure to append an event should not crash critical reconciliation unless a caller explicitly chooses that behavior.

## Severity

Supported severities:

- `info`
- `warning`
- `error`

## CLI

```sh
orch events
orch events --service api
orch events --follow
```

`--service` accepts a service name or ID. `--follow` polls the API and requests events after the most recent timestamp seen by the CLI.

## API Filters

`GET /v1/events` supports:

- `service_id`
- `task_id`
- `node_id`
- `type`
- `severity`
- `since`
- `limit`
