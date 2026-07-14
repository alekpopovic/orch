# 16. Safe service deletion and cleanup

```text
Implement safe service deletion.

Behavior:
- DELETE /v1/services/{id} marks service as deleting.
- Reconciler creates stop directives for all tasks.
- Agents stop and remove containers.
- Once all tasks are terminal/removed, service is marked deleted.
- Hard delete should not be default.

Requirements:
- Add service status:
  - active
  - deleting
  - deleted
- Add task desired status:
  - running
  - stopped
  - removed
- Add tests:
  - deleting service with running tasks
  - repeated delete request is idempotent
  - agent offline during deletion
  - final cleanup after agent returns
- Emit events.
```
