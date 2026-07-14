# 63. Policy engine design

```text
Create a design document for a policy engine.

Context:
The orchestrator needs production controls for container security, resource limits, host mounts, image registries, privileged mode, secrets, and deployment rules.

Task:
Create docs/POLICY_ENGINE_DESIGN.md.

Cover:
- goals
- non-goals
- policy enforcement points:
  - service creation
  - rollout
  - scale
  - node drain
  - secret usage
  - registry credential usage
  - agent task execution
- policy examples:
  - require resource limits
  - block privileged containers
  - allow only approved registries
  - block latest tag in production
  - require healthchecks
  - restrict host path mounts
  - restrict public ports
- static config policy vs dynamic policy
- future OPA/Rego integration
- audit logging
- test strategy

Do not implement the policy engine yet.
Design it so it fits the current validation and admission flow.
```
