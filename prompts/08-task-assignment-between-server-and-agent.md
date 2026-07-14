# 08. Task assignment between server and agent

```text
Implement task assignment between control plane and worker agents.

Server:
- Add endpoint:
  - GET /v1/agent/tasks?node_id=<id>
  - POST /v1/agent/tasks/{task_id}/status
- The GET endpoint should return desired tasks assigned to that node.
- The status endpoint should allow agent to report:
  - pulling
  - created
  - running
  - unhealthy
  - failed
  - stopped
  - removed

Agent:
- Poll for assigned tasks.
- For each task:
  - pull image
  - create container
  - start container
  - report status after each step
- Reconcile local Docker state with assigned tasks.
- Stop/remove containers that are managed by this orchestrator but no longer assigned to this node.

Requirements:
- Idempotent task execution.
- No duplicate container per task.
- Safe retry after process restart.
- Add local cache only if needed; server remains source of truth.
- Add table-driven unit tests for task state transitions.
- Add integration test with fake runtime.
```
