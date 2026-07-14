# 89. Scale test: 1,000 services with fake agents

```text
Create a scale test for 1,000 services using fake agents.

Context:
We need to understand MVP scalability limits without requiring 1,000 real containers.

Task:
Add a scale test command or integration test.

Scenario:
- start server with PostgreSQL
- register configurable number of fake nodes
- create 1,000 services with 1-5 replicas each
- simulate agents transitioning tasks to running
- measure time to convergence
- collect scheduler/reconciler metrics

Requirements:
- No real Docker required.
- Configurable:
  - service count
  - node count
  - replicas per service
  - failure rate
- Output:
  - convergence duration
  - tasks created
  - scheduler iterations
  - reconciliation iterations
  - errors
- Add docs/SCALE_TESTING.md.
- Do not run this in normal CI unless marked as optional.
```
