# 54. Autoscaling design doc

```text
Create a design document for autoscaling, but do not implement it yet.

Context:
Future orchestrator versions should support autoscaling based on CPU, memory, queue depth, or custom metrics.

Task:
Create docs/AUTOSCALING_DESIGN.md.

Cover:
- goals
- non-goals
- metrics sources
- scaling targets
- min/max replicas
- cooldown periods
- stabilization windows
- scale up/down policies
- interaction with rollout
- interaction with node capacity
- failure modes
- API proposal
- YAML proposal
- test strategy

Important:
- Do not implement autoscaling yet.
- Make the design compatible with the current reconciler and scheduler.
```
