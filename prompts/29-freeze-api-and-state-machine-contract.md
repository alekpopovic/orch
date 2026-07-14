# 29. Freeze API and state-machine contract

```text
Review and formalize the orchestrator API and state-machine contracts.

Context:
This repository implements a Docker-based container orchestrator in Go with:
- orch-server control plane
- orch-agent worker agent
- orch CLI
- PostgreSQL store
- Docker Engine runtime
- scheduler
- reconciler
- rollout controller

Task:
Create a stable internal contract for:
1. Service lifecycle
2. Task lifecycle
3. Node lifecycle
4. Deployment/Rollout lifecycle
5. Agent task execution lifecycle

Deliverables:
- docs/STATE_MACHINES.md
- Go enum definitions for all lifecycle states
- validation helpers for legal state transitions
- unit tests for allowed and forbidden transitions
- comments explaining terminal states and retryable states

Important rules:
- State transitions must be explicit.
- Invalid transitions must return domain errors.
- Terminal states must not be mutated except through documented recovery paths.
- Reconciler, scheduler, agent, and rollout controller must use the same transition helpers.
- Do not introduce large new features.
- Focus on correctness and consistency.

At the end:
- Run go test ./...
- Fix any compile or test failures.
```
