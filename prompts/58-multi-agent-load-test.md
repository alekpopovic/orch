# 58. Multi-agent load test

```text
Create a load test for multi-node orchestration behavior.

Task:
Add a load test tool under tools/loadtest or cmd/orch-loadtest.

Scenario:
- create N fake nodes
- deploy M services
- each service has R replicas
- simulate task transitions to running
- randomly fail tasks
- randomly mark nodes offline/online
- measure convergence time

Requirements:
- Use fake runtime/fake agents where possible.
- Configurable parameters:
  - nodes
  - services
  - replicas
  - failure rate
  - duration
- Output summary:
  - tasks created
  - tasks running
  - tasks failed
  - average convergence time
  - scheduler errors
  - reconciler errors
- Do not require real Docker.
- Add docs/LOAD_TESTING.md.
```
