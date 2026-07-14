# 66. Resource quotas

```text
Implement namespace-level resource quotas.

Context:
After namespaces/projects exist, each namespace should have configurable limits.

Task:
Add quotas for namespace/project resource usage.

Quota fields:
- max services
- max replicas
- max CPU millicores
- max memory bytes
- max public ports
- max secrets
- max registry credentials

Requirements:
- Quota is checked during:
  - service create
  - scale
  - rollout if it changes resources
  - secret create
  - registry credential create
- Quota accounting must be transaction-safe.
- Return clear 403 or 409 structured errors when quota is exceeded.
- Add CLI:
  - orch quota get --namespace <name>
  - orch quota set --namespace <name> ...
- Add tests:
  - quota allow
  - quota deny
  - concurrent scale requests cannot exceed quota
  - deleted services release quota
- Update docs/MULTI_TENANCY.md and docs/API.md.

At the end:
- Run go test ./...
```
