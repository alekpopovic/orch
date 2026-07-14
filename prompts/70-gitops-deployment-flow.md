# 70. GitOps deployment flow

```text
Implement a GitOps-style sync controller MVP.

Context:
Operators may want the orchestrator to reconcile service definitions from a Git repository instead of manually running orch deploy.

Task:
Add GitOps MVP.

Spec:
- A GitOps source points to:
  - repository URL
  - branch
  - path
  - sync interval
  - namespace
- The controller periodically fetches specs and applies them.
- Deleting a file should optionally delete the service depending on prune setting.

Requirements:
- Add GitOpsSource model and store.
- Add controller loop with context cancellation.
- Add API:
  - POST /v1/gitops/sources
  - GET /v1/gitops/sources
  - DELETE /v1/gitops/sources/{id}
  - POST /v1/gitops/sources/{id}/sync
- Add CLI:
  - orch gitops add
  - orch gitops ls
  - orch gitops sync
  - orch gitops delete
- Validate YAML through existing admission path.
- Emit events and audit logs.
- Add tests using a local fixture repo.
- Do not store Git credentials in plaintext.
- Update docs/GITOPS.md.

At the end:
- Run go test ./...
```
