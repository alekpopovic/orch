# 64. Admission controller MVP

```text
Implement an admission controller MVP for service specs.

Context:
The orchestrator needs a central place to validate and reject unsafe or non-compliant workload definitions before they are persisted.

Task:
Add an admission package used by service create, rollout, and update paths.

Requirements:
- Create internal/admission package.
- Admission should receive:
  - actor
  - operation type
  - namespace/project if implemented
  - service spec
  - cluster policy config
- Enforce:
  - required resource requests and limits
  - no privileged containers unless allowed
  - no host network unless allowed
  - no host PID unless allowed
  - allowed image registries
  - block image tag "latest" if configured
  - require healthcheck if configured
  - allowed host path prefixes
  - max replicas per service
  - max public ports per service
- Return structured errors.
- Emit audit event for rejected admission.
- Add tests for every policy.
- Update docs/SECURITY.md and docs/SERVICE_SPEC.md.

At the end:
- Run go test ./...
```
