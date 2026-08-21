# Changelog

## v0.3.0 - 2026-08-21

Compatibility, policy, multi-tenancy, workload, and operational-hardening release.

### Added

- Stable OpenAPI v1 contract, API-version compatibility headers, and public Go client.
- Admission policy, namespace isolation, transaction-safe resource quotas, image digest pinning, and GitOps reconciliation/drift controls.
- One-shot and cron jobs, persistent-volume abstractions, internal DNS, and notification sinks.
- Maintenance windows with audited emergency bypass, agent/server/schema compatibility metadata, advisory-locked migrations, retention pruning, and namespace usage export.
- Parser fuzz targets, randomized scheduler invariants, controller/API benchmarks, isolated opt-in pprof, and an optional 1,000-service fake-agent scale scenario.

### Changed

- Agents report their version during registration and heartbeat; agents below `0.2.0` are rejected and versions above `0.3.0` receive a warning.
- Database schema version is 16; supported startup range is 15–16 when schema checking is enabled.
- `orch version` now reports `0.3.0`, and cluster/migration preflight commands expose upgrade state.

### Known Boundaries

- The default server still uses an in-memory control plane; PostgreSQL store wiring and restart-safe production state remain future work.
- Advanced CNI, stateful scheduling, external plugins, image signing, vulnerability scanning, payments, and invoicing remain design-only or out of scope.

### Verification

- `go test ./...`, lint/vet, OpenAPI validation, migration safety tests, and security-sensitive grep checks are release gates.

## v0.2.0 - 2026-07-14

Production-hardening release for the MVP orchestrator.

### Added

- Prometheus alert rules and Grafana dashboard artifacts.
- Backup and restore scripts and runbook.
- Production Docker Compose and systemd deployment examples.
- Security review with high-risk healthcheck SSRF fix.
- Default container security context and cluster policy enforcement.
- Healthcheck scheme/path validation, redirect blocking, and response body read limits.
- Autoscaling design and CPU-based autoscaler MVP with fake metrics provider.
- HA control-plane design and PostgreSQL advisory-lock leader election MVP.
- Multi-agent load test command and chaos-style integration scenarios.
- GitHub Pages documentation portal with release docs, OpenAPI access, sidebar navigation, and auto/light/dark theme support.
- Branded docs refresh with a unique SVG logo system, colorful README landing page, icons, and Mermaid charts.

### Changed

- Service specs now include `security_context` and `autoscaling`.
- Agent healthchecks only target assigned published TCP ports.
- Docker runtime drops `NET_RAW` by default unless an explicit capability drop list is provided.
- Prometheus metrics include autoscaler and controller-leadership signals.

### Verification

- `go test ./...` passes.
- `go build ./...` passes.
- `docker compose config` passes in the local environment.
- Security-sensitive grep checks found no production secret/token logging blockers.
- `golangci-lint` was not installed in the local environment.

## v0.1.0 - 2026-07-14

Initial MVP release with domain models, HTTP API, CLI, agent registration, task assignment, scheduler, reconciler, healthchecks, logs, events, rollout/rollback, resource accounting, auth/RBAC, metrics, local development, CI, and production documentation.
