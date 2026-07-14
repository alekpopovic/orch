# 09. Scheduler v1

```text
Implement scheduler v1.

Goal:
Assign pending tasks to suitable ready nodes.

Inputs:
- service resource requests
- service placement labels
- current node status
- current running tasks
- node capacity and allocatable resources

Algorithm:
1. Load pending tasks.
2. Load ready nodes.
3. Filter nodes:
   - status must be ready
   - not draining
   - labels must match placement constraints
   - enough free CPU/memory
4. Score nodes:
   - prefer more free memory
   - prefer fewer running tasks for same service
   - prefer fewer total running tasks
5. Assign task to best node.
6. Emit event.

Requirements:
- Scheduler must be deterministic.
- Add interfaces for store access.
- No direct Docker calls in scheduler.
- Add table-driven unit tests covering:
  - no nodes
  - labels mismatch
  - insufficient resources
  - spread across nodes
  - deterministic tie-breaking
  - node draining
- Add docs/SCHEDULER.md with algorithm explanation.
```
