# 75. Stateful workloads design

```text
Create a design document for stateful workloads.

Context:
The orchestrator currently focuses on stateless services. Stateful workloads require stable identity, stable storage, and careful replacement semantics.

Task:
Create docs/STATEFUL_WORKLOADS_DESIGN.md.

Cover:
- goals
- non-goals
- stable task identity
- ordered startup/shutdown
- persistent volume claims
- node affinity
- backup/snapshot hooks
- replacement behavior when node is offline
- split-brain risks
- service discovery for stateful tasks
- rollout strategy for stateful workloads
- API/YAML proposal
- scheduler changes
- agent changes
- test strategy

Do not implement stateful workloads yet.
Be conservative and clearly mark risks.
```
