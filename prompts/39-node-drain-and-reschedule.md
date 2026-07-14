# 39. Node drain and reschedule

```text
Implement production-grade node drain behavior.

Context:
Operators need to safely remove a node from scheduling and move workloads elsewhere.

Command:
orch node drain <node-id>

Behavior:
- Mark node as draining.
- Scheduler must stop placing new tasks on it.
- Reconciler should create replacement tasks on other ready nodes.
- Once replacements are healthy, old tasks on drained node should be stopped.
- If no capacity exists, drain should remain pending and emit warning events.
- Add uncordon support to return node to ready state.

Requirements:
- Add drain status/progress.
- Add API:
  - POST /v1/nodes/{id}/drain
  - POST /v1/nodes/{id}/uncordon
  - GET /v1/nodes/{id}/drain-status
- Add CLI:
  - orch node drain
  - orch node uncordon
- Add tests:
  - drain with enough capacity
  - drain with insufficient capacity
  - uncordon before completion
  - node offline during drain
- Update docs/OPERATIONS.md with runbook.
```
