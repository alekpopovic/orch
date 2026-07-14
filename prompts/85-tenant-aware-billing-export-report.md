# 85. Tenant-aware billing/export report

```text
Implement usage export for namespaces/projects.

Context:
For internal platforms or SaaS-style usage, operators need usage reports by namespace/project.

Task:
Add usage accounting export.

Metrics:
- CPU requested over time
- memory requested over time
- replica count over time
- service count
- task runtime duration
- public ports
- storage claims if implemented

Requirements:
- Add periodic usage snapshots.
- Add API:
  - GET /v1/usage?namespace=&from=&to=
- Add CLI:
  - orch usage --namespace <name> --from <date> --to <date>
  - orch usage export --format csv
- Add CSV export.
- Add tests for usage aggregation.
- Update docs/MULTI_TENANCY.md and docs/OPERATIONS.md.

Do not implement payments or invoicing.
```
