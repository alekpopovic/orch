# 31. Scheduler concurrency safety

```text
Make scheduler safe under concurrent execution.

Context:
In the future there may be multiple orch-server instances or accidental multiple scheduler loops. Scheduler must not assign the same pending task twice.

Task:
Update scheduler and store layer so task assignment is concurrency-safe.

Requirements:
- Pending tasks should be claimed atomically.
- Two scheduler loops must not assign the same task to different nodes.
- Prefer SELECT FOR UPDATE SKIP LOCKED or equivalent PostgreSQL-safe pattern.
- Assignment must include:
  - task ID
  - selected node ID
  - assigned timestamp
  - version/update guard
- Add tests that simulate two schedulers running at the same time.
- Add deterministic tie-breaking when nodes have the same score.
- Add metrics for:
  - tasks claimed
  - assignment conflicts
  - scheduling attempts
  - scheduling failures
- Document scheduler concurrency assumptions in docs/SCHEDULER.md.

Do not implement full HA leader election yet.
Focus only on correctness if scheduler accidentally runs concurrently.
```
