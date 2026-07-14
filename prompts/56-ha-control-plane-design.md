# 56. HA control plane design

```text
Create a design document for high-availability control plane.

Context:
The orchestrator currently has one orch-server instance. Future production should support multiple control-plane instances.

Task:
Create docs/HA_CONTROL_PLANE_DESIGN.md.

Cover:
- goals
- non-goals
- active-active vs active-passive
- leader election needs
- which controllers need leader locks:
  - scheduler
  - reconciler
  - rollout controller
  - node monitor
  - autoscaler
- database locking options
- etcd/Raft option
- PostgreSQL advisory locks option
- failure modes:
  - leader dies
  - split brain
  - DB unavailable
  - network partition
- migration path from current architecture
- testing strategy

Do not implement HA yet.
Recommend the simplest safe MVP approach.
```
