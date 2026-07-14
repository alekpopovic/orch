# 00. AGENTS.md and architecture docs

```text
Create an AGENTS.md file for this repository.

Project context:
We are building a production-grade Docker container orchestrator in Go. The orchestrator should manage multi-node deployments using Docker Engine API first, with a clean architecture that could later support containerd.

Core components:
- orch-server: control plane API server
- orch-agent: worker node agent
- orch: CLI
- scheduler: places tasks on nodes
- reconciler: keeps actual state equal to desired state
- store: PostgreSQL persistence
- docker runtime package: wrapper around Docker Engine API
- healthchecker: HTTP/TCP health checks
- rollout manager: rolling updates and rollback
- events/logs/metrics

Create AGENTS.md with:
- repository layout
- Go coding conventions
- error handling conventions
- logging conventions
- testing expectations
- migration rules
- API compatibility rules
- security rules
- commands for build/test/lint
- definition of done for every task

Assume:
- Go modules
- PostgreSQL
- Docker Engine API
- Cobra CLI
- sqlc or pgx for database access
- golangci-lint
- Prometheus metrics

Important constraints:
- Prefer small cohesive packages.
- No global mutable state unless explicitly justified.
- All external side effects must be behind interfaces.
- Scheduler and reconciler must be deterministic and unit-testable.
- Docker operations must be idempotent.
- Never store secrets in plaintext.
- Every feature must include tests.
- Every API endpoint must have request/response structs and validation.
- Every long-running loop must support context cancellation.
- All timestamps must be UTC.

After creating AGENTS.md, also create a short docs/ARCHITECTURE.md summarizing the system.
```
