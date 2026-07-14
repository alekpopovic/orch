# 18. User API auth v1

```text
Implement user API authentication and authorization v1.

Scope:
- JWT-based auth for user-facing REST API.
- Static user config or database-backed users for MVP.
- Roles:
  - admin
  - operator
  - viewer

Permissions:
- viewer:
  - list/read nodes, services, tasks, events, logs
- operator:
  - deploy, scale, rollout, rollback, delete services
- admin:
  - manage users, drain nodes, configure cluster

Requirements:
- Add auth middleware.
- Add RBAC middleware.
- Keep agent authentication separate from user authentication.
- Never log tokens.
- Add tests for:
  - missing token
  - invalid token
  - insufficient role
  - allowed access
- Update docs/SECURITY.md.
```
