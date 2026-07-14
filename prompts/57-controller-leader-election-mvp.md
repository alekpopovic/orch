# 57. Controller leader election MVP

```text
Implement MVP leader election for controllers using PostgreSQL advisory locks.

Context:
We need to prevent multiple orch-server instances from running the same controller concurrently.

Scope:
- scheduler
- reconciler
- rollout controller
- node monitor
- autoscaler if implemented

Requirements:
- Add LeaderElector interface.
- Add PostgreSQL advisory lock implementation.
- Controllers should run only while lock is held.
- If lock is lost, controller loop must stop safely.
- Add metrics:
  - leader status
  - lock acquisition failures
- Add tests for:
  - one leader active
  - second instance waits/fails
  - lock released on shutdown
  - controller stops when lock lost
- Update docs/HA_CONTROL_PLANE_DESIGN.md and docs/OPERATIONS.md.

Do not introduce etcd yet.
```
