# 13. Durable events system

```text
Implement a durable events system.

Events should be emitted by:
- API service creation/update/delete
- scheduler task assignment
- reconciler task creation/stop decisions
- agent task status changes
- health checker unhealthy transitions
- rollout manager

Requirements:
- Store events in PostgreSQL.
- Add event severity:
  - info
  - warning
  - error
- Add event type constants.
- Add helper function:
  Emit(ctx, Event)
- Do not let event emission failure crash critical paths unless explicitly configured.
- Add API filtering:
  - by service_id
  - by task_id
  - by node_id
  - by type
  - by severity
  - since timestamp
- Add CLI:
  orch events
  orch events --service api
  orch events --follow
- Add tests.
```
