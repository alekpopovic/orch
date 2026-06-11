# Events

Events are durable orchestration facts stored in PostgreSQL. They are emitted best-effort by default: critical control-plane paths should continue even if event storage is temporarily unavailable.

## Sources

Events are emitted by:

- API/control-plane service create, scale, rollout, rollback, and delete operations.
- scheduler task assignments.
- reconciler task creation and stop decisions.
- agent task status reports.
- healthcheck unhealthy transitions.
- rollout manager start decisions.

## Severity

Supported severities are:

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
