# 30. Transaction boundaries audit

```text
Audit and improve database transaction boundaries.

Context:
This orchestrator has multiple controllers:
- reconciler creates/stops tasks
- scheduler assigns tasks
- rollout controller changes service versions
- agents report task status
- API handlers create/update desired state

Task:
Review all store operations and identify places where multiple writes must be atomic.

Implement transaction support for critical flows:
- service creation + initial version creation + event emission
- scale request + event emission
- task assignment by scheduler
- agent task status update + event emission
- rollout creation + service version update
- rollback creation
- service deletion marking

Requirements:
- Add store.WithTx(ctx, fn) or equivalent transaction abstraction.
- Avoid leaking database-specific transaction types into domain logic.
- Prevent partial writes that could corrupt desired/actual state.
- Use row-level locking where needed to avoid double scheduling.
- Add tests for rollback-on-error.
- Add tests for concurrent scheduler attempts assigning the same task.
- Document transaction rules in docs/RELIABILITY.md.

At the end:
- Run go test ./...
- Fix failures.
```
