# 87. Property tests for scheduler

```text
Add property-style tests for scheduler correctness.

Context:
The scheduler is critical and must maintain invariants across many random cluster states.

Task:
Create randomized tests for scheduler.

Invariants:
- task assigned only to ready node
- task not assigned to draining/offline node
- assigned node has enough CPU/memory
- placement labels match
- host port conflicts are avoided
- deterministic output for same input
- no duplicate assignment

Requirements:
- Generate random nodes, tasks, services, labels, and resources.
- Use fixed seeds for reproducibility.
- Log seed on failure.
- Keep tests fast.
- Add docs/TESTING.md section.

At the end:
- Run go test ./...
```
