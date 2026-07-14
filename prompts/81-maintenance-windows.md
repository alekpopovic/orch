# 81. Maintenance windows

```text
Implement maintenance windows for disruptive operations.

Context:
Production operators may want rollouts, node drains, and automatic replacements to happen only during approved windows.

Task:
Add maintenance window support.

Requirements:
- Add MaintenanceWindow model:
  - name
  - namespace or global scope
  - schedule
  - timezone
  - allowed operations
  - enabled flag
- Operations that can respect windows:
  - rollout
  - rollback
  - node drain
  - autoscaling scale-down if implemented
  - non-urgent task replacement
- Emergency operations may bypass with --force and audit log.
- Add API and CLI:
  - orch maintenance create
  - orch maintenance ls
  - orch maintenance delete
- Add tests using fake clock.
- Update docs/OPERATIONS.md.

Do not block critical safety replacements unless explicitly configured.
```
