# Changelog

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
