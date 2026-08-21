# orch v0.3.0 Release Notes

v0.3.0 turns the MVP into a substantially stronger platform contract. The release adds a documented, stable v1 API; namespace-aware policy and quota boundaries; GitOps and scheduled/one-shot workloads; and explicit operational safety for upgrades, migrations, disruptive maintenance, retention, and capacity testing.

## Highlights

- Namespace isolation covers services, tasks, deployments, events, secrets, registry credentials, audit history, maintenance windows, and usage reports.
- Admission rejects unsafe workload specs before mutation; quotas account for replicas, requested CPU/memory, public ports, secrets, and registry credentials.
- GitOps supports deterministic sync, drift reporting, and an opt-in auto-revert policy. Jobs, cron jobs, volume claims, internal DNS, and notification sinks extend the workload surface.
- `GET /v1/version` and `orch cluster check-upgrade` report server, API, agent, and schema compatibility. Agents report their version on registration and heartbeat.
- `orch-server migrate status|up|down` records versions, serializes migrations with an advisory lock, and requires explicit authorization for down migrations.
- Maintenance windows govern rollout, rollback, drain, and scale-down operations. Emergency `--force` bypasses are audited.
- Retention pruning protects active/unresolved state, while namespace usage snapshots export CPU, memory, replica, service, task-runtime, public-port, and storage-claim measurements as JSON or CSV.
- New fuzz, scheduler property, benchmark, profiling, and optional 1,000-service scale suites improve confidence and provide reproducible diagnostics.

## Compatibility and Upgrade

The server version is `0.3.0`, API version is `v1`, and database schema is 16. Supported agents are `0.2.0`–`0.3.0`; newer agents are accepted with an untested-version warning. Supported schema versions are 15–16. Follow [docs/UPGRADES.md](docs/UPGRADES.md) and [docs/MIGRATIONS.md](docs/MIGRATIONS.md), back up PostgreSQL first, migrate before replacing the server, and roll agents node by node.

## Important Boundary

The default `orch-server` runtime still owns live control-plane state in memory even though the PostgreSQL store and migrations exist. Do not treat v0.3.0 as restart-safe durable orchestration until that store is wired into the server. Advanced networking, stateful scheduling, external plugin execution, signing/scanning enforcement, and billing are not included.

## Verification

The release gate covers the full Go suite, vet/lint, OpenAPI parsing and path coverage, migration compatibility/locking/idempotency tests, and secret-sensitive source scans. The optional scale suite remains excluded from normal CI by its `scale` build tag.
