# 32. Reconciler idempotency under restart

```text
Harden the reconciler for server restarts and repeated runs.

Context:
The reconciler keeps desired service replicas equal to actual tasks. It may be interrupted at any time.

Task:
Make reconciler idempotent and restart-safe.

Requirements:
- Repeated reconciliation must not create duplicate tasks.
- Failed tasks must produce at most one replacement unless policy allows multiple attempts.
- Scale-down selection must be stable and deterministic.
- Deleted services must continue cleanup after server restart.
- Rollout-owned tasks must not be incorrectly counted as normal tasks.
- Add reconciliation generation or operation ID if needed.
- Add tests for:
  - server restart between task creation and scheduling
  - repeated reconcile loop with same service
  - failed task replacement only once
  - scale down repeated twice
  - service deletion repeated twice
- Emit events only when a real state change happens.
- Update docs/RECONCILER.md.

At the end:
- Run go test ./...
```
