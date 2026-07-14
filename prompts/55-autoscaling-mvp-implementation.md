# 55. Autoscaling MVP implementation

```text
Implement autoscaling MVP based on the accepted docs/AUTOSCALING_DESIGN.md.

Scope:
- CPU-based autoscaling using metrics abstraction
- fake metrics provider for tests
- optional Prometheus provider if straightforward

Service YAML example:

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilization: 70
  cooldown: 60s

Requirements:
- Add autoscaler controller.
- Autoscaler updates desired replicas through the same service scale path as CLI/API.
- Respect min/max replicas.
- Respect cooldown.
- Do not scale during active rollout unless design allows it.
- Emit events for scale decisions.
- Add metrics for autoscaler decisions.
- Add tests:
  - scale up
  - scale down
  - cooldown
  - max replicas
  - min replicas
  - active rollout interaction
- Update docs.
```
