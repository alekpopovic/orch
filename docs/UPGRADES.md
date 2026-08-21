# Cluster Upgrades

orch v0.3.0 publishes the server, API, agent, and database compatibility range at `GET /v1/version`. Before changing any component, run:

```sh
orch version
orch cluster check-upgrade
orch-server migrate status --database-url "$DATABASE_URL"
```

The v0.3.0 server supports agents from `0.2.0` through `0.3.0`. Older agents are rejected at registration and heartbeat. A newer agent may connect, but receives a `version_warning` directive because the combination has not been tested. The supported database schema range is 15–16.

## Upgrade Order

1. Back up PostgreSQL and verify the restore procedure.
2. Put disruptive workload operations in an approved maintenance window if required.
3. Stop all but one control-plane replica from running migrations.
4. Run `orch-server migrate status`, then `orch-server migrate up`. The migration runner also takes a PostgreSQL advisory lock.
5. Upgrade the server and check `/readyz` and `/v1/version`.
6. Roll agents one node at a time: drain, replace the agent binary/image, wait for a healthy heartbeat, then uncordon.
7. Upgrade the CLI after the server. The v1 API remains backward compatible.

## Rollback

Prefer rolling the server binary back while leaving an additive schema in place. Run a down migration only when its release notes explicitly call it safe, after a backup, and with `orch-server migrate down --allow-down`. If an agent rollback falls below the advertised minimum, restore a compatible server first. A failed server startup caused by an old or future schema must be resolved by applying a supported migration or restoring the matching release; never bypass the compatibility check in production.

