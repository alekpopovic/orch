# 03. PostgreSQL schema and store layer

```text
Implement the PostgreSQL persistence layer.

Create migrations for:
- nodes
- services
- service_versions if needed
- tasks
- deployments
- events

Requirements:
- Use UUID primary keys.
- Store timestamps in UTC.
- Add indexes for:
  - tasks by service_id
  - tasks by node_id
  - tasks by status
  - nodes by status
  - events by related object and created_at
- Add optimistic concurrency where appropriate, for example version column or updated_at checks.
- Add store interfaces for:
  - NodeStore
  - ServiceStore
  - TaskStore
  - DeploymentStore
  - EventStore
- Implement PostgreSQL-backed stores.
- Add unit/integration tests.
- Integration tests may use testcontainers-go or a docker-compose documented flow.
- All store operations must accept context.Context.
- No SQL string concatenation with untrusted input.
- Return domain-specific errors:
  - ErrNotFound
  - ErrConflict
  - ErrInvalidState
  - ErrDuplicate

At the end:
- Document how to run migrations.
- Run tests and fix failures.
```
