# 41. Manual Docker drift reconciliation

```text
Handle drift caused by manual Docker operations.

Context:
An operator may manually stop/remove containers with docker CLI. The orchestrator must detect and repair drift.

Task:
Improve agent local reconciliation.

Agent should:
- List managed containers using orch labels.
- Compare local containers with assigned tasks from server.
- Report missing containers.
- Stop/remove managed containers no longer assigned to this node.
- Detect containers with mismatched image/version/labels.
- Avoid touching non-orchestrator containers.

Requirements:
- Add drift event types:
  - managed_container_missing
  - unexpected_managed_container
  - managed_container_state_mismatch
- Add tests using FakeRuntime:
  - container manually removed
  - container manually stopped
  - unexpected managed container exists
  - non-managed container ignored
- Update docs/AGENT.md.
```
