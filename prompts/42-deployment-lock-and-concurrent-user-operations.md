# 42. Deployment lock and concurrent user operations

```text
Protect services from unsafe concurrent user operations.

Context:
Users may call scale, rollout, rollback, and delete at the same time.

Task:
Add service-level operation locking.

Rules:
- delete wins over scale/rollout.
- rollback cannot start during active rollout unless forced.
- rollout cannot start during another active rollout.
- scale during rollout should either be supported explicitly or rejected with clear error.
- repeated same operation should be idempotent when possible.

Requirements:
- Add operation lock or service operation status.
- Return clear API errors:
  - 409 conflict
  - operation already in progress
- Add tests:
  - rollout + rollout
  - rollout + rollback
  - delete + rollout
  - scale + delete
  - repeated delete
- Update docs/API.md and docs/ROLLOUTS.md.
```
