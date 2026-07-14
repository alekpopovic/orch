# 01. Repository bootstrap

```text
Initialize the repository for a Go-based Docker container orchestrator.

Create this structure:

cmd/
  orch/
    main.go
  orch-server/
    main.go
  orch-agent/
    main.go

internal/
  api/
  auth/
  config/
  docker/
  events/
  health/
  node/
  reconciler/
  rollout/
  scheduler/
  store/
  task/

pkg/
  types/

deployments/
  examples/

docs/
  ARCHITECTURE.md
  DEVELOPMENT.md

migrations/

Requirements:
- Initialize go.mod.
- Create basic buildable binaries for orch, orch-server, and orch-agent.
- Add a Makefile with:
  - make build
  - make test
  - make lint
  - make run-server
  - make run-agent
- Add docker-compose.yml with PostgreSQL and optional local orchestrator services.
- Add .env.example.
- Add README.md with quickstart.
- Add basic structured logging.
- Add context cancellation handling in all main packages.
- Add graceful shutdown for server and agent.
- Add unit test placeholders where useful.

Use idiomatic Go.
Keep the initial code minimal but production-shaped.
At the end, run go test ./... and fix failures.
```
