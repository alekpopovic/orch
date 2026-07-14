# 65. Namespaces / projects

```text
Implement namespaces or projects for workload isolation.

Context:
The orchestrator currently has global services, nodes, secrets, events, and users. Production usage needs logical separation.

Task:
Add a Namespace or Project model.

Requirements:
- Add namespace/project table.
- Add namespace field to:
  - services
  - tasks
  - deployments
  - events
  - secrets
  - registry credentials
  - audit logs
- CLI should support:
  - --namespace
  - default namespace from config
  - orch namespace ls
  - orch namespace create <name>
  - orch namespace delete <name>
- API should scope list/read/write operations by namespace.
- RBAC should support namespace-scoped roles.
- Existing data should migrate to default namespace.
- Add tests:
  - namespace isolation
  - same service name allowed in different namespaces
  - secret references cannot cross namespace unless explicitly allowed
  - viewer in one namespace cannot read another namespace
- Update docs/MULTI_TENANCY.md.

At the end:
- Run migrations and go test ./...
```
