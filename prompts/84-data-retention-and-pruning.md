# 84. Data retention and pruning

```text
Implement data retention and pruning policies.

Context:
Events, audit logs, task history, rollout history, and logs metadata can grow without bound.

Task:
Add retention configuration and pruning controller.

Retention targets:
- events
- audit logs
- completed tasks
- failed tasks
- old rollouts
- completed jobs
- old GitOps sync records

Requirements:
- Add retention config.
- Add pruning controller with dry-run mode.
- Add CLI:
  - orch retention status
  - orch retention prune --dry-run
- Never prune active services, running tasks, active rollouts, or unresolved failures.
- Emit audit logs for pruning.
- Add tests:
  - prune old events
  - keep recent events
  - keep active task history
  - dry-run does not delete
- Update docs/OPERATIONS.md.
```
