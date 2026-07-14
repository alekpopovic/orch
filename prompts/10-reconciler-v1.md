# 10. Reconciler v1

```text
Implement the service reconciler.

Goal:
Keep actual task count equal to desired service replicas.

Loop:
- List active services.
- For each service:
  - Count non-terminal tasks for current service version.
  - If count < replicas, create missing pending tasks.
  - If count > replicas, select extra tasks to stop.
  - If tasks are failed and restart policy allows restart, create replacement tasks.
- Emit events for every decision.

Requirements:
- All logic must be idempotent.
- Use context cancellation.
- Add configurable interval.
- Add leader-lock abstraction but do not implement HA locking yet.
- Do not schedule directly from reconciler; create pending tasks and let scheduler assign them.
- Add unit tests for:
  - scale up
  - scale down
  - failed task replacement
  - no-op when desired equals actual
  - service deletion
  - version mismatch
- Add metrics:
  - reconciliation duration
  - reconciliation errors
  - created tasks count
  - stopped tasks count
```
