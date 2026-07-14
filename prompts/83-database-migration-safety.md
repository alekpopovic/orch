# 83. Database migration safety

```text
Harden database migrations for production.

Context:
The orchestrator depends on PostgreSQL migrations. Unsafe migrations can break production control plane.

Task:
Improve migration tooling and safety.

Requirements:
- Add migration status command:
  - orch-server migrate status
  - orch-server migrate up
  - orch-server migrate down only if explicitly allowed
- Add schema version table if not present.
- Add advisory lock around migrations.
- Add startup check that refuses to run with unsupported schema version.
- Add docs/MIGRATIONS.md.
- Add tests for:
  - migration lock
  - startup with old schema
  - startup with future schema
  - idempotent migration status
- Ensure migrations are reversible only when safe.
```
