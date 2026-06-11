# AGENTS.md

This repository builds `orch`, a production-grade Docker container orchestrator written in Go. The system manages multi-node deployments through the Docker Engine API first, while keeping runtime boundaries clean enough to support containerd later.

These instructions apply to the whole repository unless a deeper `AGENTS.md` overrides them.

## Repository Layout

Use this intended layout as the project grows:

```text
cmd/
  orch-server/          # Control plane API server entrypoint
  orch-agent/           # Worker node agent entrypoint
  orch/                 # Cobra CLI entrypoint
internal/
  api/                  # HTTP handlers, routes, middleware, request/response DTOs
  scheduler/            # Deterministic task placement logic
  reconciler/           # Desired-state to actual-state convergence loops
  store/                # Persistence interfaces and PostgreSQL implementations
  runtime/
    docker/             # Docker Engine API adapter
  healthchecker/        # HTTP and TCP health checks
  rollout/              # Rolling update and rollback coordination
  events/               # Event publishing and subscriptions
  logs/                 # Log collection and streaming boundaries
  metrics/              # Prometheus collectors and instrumentation helpers
  config/               # Configuration loading and validation
  auth/                 # Authentication, authorization, and secret handling
pkg/                    # Public packages only when external consumers need them
migrations/             # PostgreSQL schema migrations
api/                    # OpenAPI specs or API contract artifacts
docs/                   # Architecture and operator documentation
test/                   # Integration test fixtures and helpers
```

Prefer `internal/` for application packages. Add `pkg/` only for stable, externally consumable libraries.

## Go Coding Conventions

- Use Go modules.
- Keep packages small, cohesive, and named after the behavior they own.
- Avoid package-level mutable state. Any global value must be immutable or explicitly justified in code review.
- Pass dependencies explicitly through constructors.
- Put all external side effects behind interfaces, including Docker, PostgreSQL, clocks, network probes, event sinks, metrics sinks, and log streams.
- Keep interfaces close to the consumer package unless they represent a shared domain boundary.
- Long-running loops must accept `context.Context`, check cancellation promptly, and return cleanly.
- All timestamps must be UTC. Inject clocks in deterministic code and tests.
- Scheduler and reconciler logic must be deterministic and unit-testable without Docker, PostgreSQL, or real time.
- Docker runtime operations must be idempotent. Repeating the same operation should converge to the same state without duplicate resources or false failures.
- Do not add broad utility packages. Prefer concrete domain helpers in the package that uses them.

## Error Handling Conventions

- Return errors; do not panic outside process startup validation or truly unrecoverable programmer errors.
- Wrap errors with actionable context using `%w`.
- Preserve sentinel/domain errors where callers need branching with `errors.Is` or `errors.As`.
- Do not log and return the same error at the same layer. Log at the boundary that handles or drops the error.
- API handlers must translate internal errors into stable, documented response shapes.
- Retriable operations must distinguish transient failures from permanent validation or authorization failures.
- Background loops must report errors through structured logs, metrics, and events where appropriate.

## Logging Conventions

- Use structured logging.
- Include stable fields such as `request_id`, `node_id`, `task_id`, `service_id`, `deployment_id`, and `container_id` when available.
- Do not log secrets, tokens, private keys, raw credentials, or full connection strings.
- Log at levels consistently:
  - `debug`: detailed diagnostics for local investigation.
  - `info`: lifecycle events and successful state transitions.
  - `warn`: recoverable degradation or retryable failure.
  - `error`: operation failed and needs operator attention.
- Prefer event records for user-visible orchestration history and logs for operator diagnostics.

## Testing Expectations

- Every feature must include tests.
- Unit tests are required for scheduler, reconciler, rollout, health checking, validation, and runtime idempotency logic.
- Tests for scheduler and reconciler must not depend on map iteration order, real time, Docker, PostgreSQL, or network access.
- Use table-driven tests for decision logic.
- Add integration tests for PostgreSQL migrations, store implementations, Docker runtime behavior, and API routes when those surfaces change.
- Use fakes or test doubles for external side effects.
- Include regression tests for bug fixes.
- Tests must be deterministic and safe to run in parallel unless explicitly documented otherwise.

## Migration Rules

- Store schema changes in `migrations/`.
- Every migration must have an up and down path unless a one-way migration is explicitly justified.
- Migrations must be backward-compatible with at least one running version during rolling deploys.
- Do not rewrite committed migrations. Add a new migration instead.
- Keep data migrations idempotent and bounded.
- Validate migrations in tests against PostgreSQL.
- Database access should use `sqlc` or `pgx`; avoid ad hoc SQL spread across unrelated packages.

## API Compatibility Rules

- Every endpoint must define explicit request and response structs.
- Every request must be validated at the API boundary before reaching domain logic.
- Use stable error response formats.
- Do not remove or rename fields in existing API responses without a versioned migration path.
- Additive response fields are preferred for compatible changes.
- Keep API contracts documented in `api/` when the endpoint is public or used by the CLI/agent.
- The CLI and agents must tolerate unknown fields from newer servers where practical.
- All API timestamps must be UTC and encoded consistently.

## Security Rules

- Never store secrets in plaintext.
- Encrypt secrets at rest and redact secrets in logs, metrics, traces, errors, and events.
- Treat Docker access as privileged. Keep Docker Engine API credentials and socket access tightly scoped.
- Validate and authorize every control plane operation.
- Use least-privilege database roles and runtime permissions.
- Do not expose internal debug endpoints without authentication.
- Sanitize user-controlled values before using them in labels, names, paths, shell commands, queries, or log fields.
- Avoid shelling out for orchestration operations; prefer typed APIs.
- Dependency changes must be reviewed for license, maintenance, and security posture.

## Build, Test, And Lint Commands

Expected commands once the repository is scaffolded:

```sh
go mod tidy
go build ./...
go test ./...
golangci-lint run ./...
```

Suggested project targets if a `Makefile` is added:

```sh
make build
make test
make lint
make migrate-up
make migrate-down
```

Integration tests that require Docker or PostgreSQL must document their prerequisites and should be separable from fast unit tests.

## Definition Of Done

Every task is done only when:

- The change is implemented in the smallest cohesive package boundaries.
- External side effects are isolated behind interfaces.
- Scheduler and reconciler changes are deterministic and unit-tested.
- Docker operations are idempotent and tested with repeated calls or equivalent fakes.
- API endpoints have request/response structs, validation, stable errors, and compatibility considered.
- Long-running loops support context cancellation.
- Timestamps are UTC.
- Secrets are encrypted or redacted as appropriate and never stored in plaintext.
- Migrations are added and tested when persistence changes.
- Prometheus metrics, events, or logs are added for operationally important behavior.
- Tests cover the feature and any regression risk.
- `go test ./...` passes.
- `golangci-lint run ./...` passes when available.
- Documentation is updated when behavior, architecture, configuration, or operator workflows change.

## Codex Workflow Rule

After every user prompt that results in repository changes, Codex must run:

```sh
git add <changed-files>
git commit -m "<concise task summary>"
git push
```

Before committing, inspect `git status` and stage only files relevant to the completed prompt. Do not include unrelated user changes in the commit.
