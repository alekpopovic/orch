# 40. Node offline detection and recovery

```text
Implement node offline detection and recovery.

Context:
Agents send heartbeat to the control plane. The server must detect stale nodes.

Task:
Add node monitor controller.

Behavior:
- If node heartbeat is stale beyond threshold, mark node offline.
- Scheduler must not place new tasks on offline nodes.
- Reconciler should decide whether to replace tasks from offline nodes.
- When node returns, agent must reconcile local containers with server assignments.
- Avoid duplicate live tasks when a network partition heals.

Requirements:
- Add configurable heartbeat timeout.
- Add node statuses:
  - ready
  - draining
  - offline
  - unknown
- Add task condition:
  - node_lost
- Add conservative replacement policy:
  - stateless services may be replaced after timeout
  - stateful services should not be replaced automatically yet
- Add tests:
  - heartbeat stale marks node offline
  - returned node reconciles stale containers
  - scheduler avoids offline node
  - replacement is created only once
- Update docs/RELIABILITY.md.
```
