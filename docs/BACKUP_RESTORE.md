# Backup And Restore

`orch` persists durable control-plane state in PostgreSQL when the server is wired to `PostgresStore`. Backups should be treated as sensitive because they include encrypted secret material and operational history.

## What Is Backed Up

`scripts/backup-db.sh` creates a PostgreSQL custom-format dump with `pg_dump`. It includes:

- desired state: services, service versions, routes, registry references, and deployments;
- actual state: nodes, tasks, task ports, and task conditions;
- rollout history;
- orchestration events;
- audit logs;
- encrypted secret values and metadata;
- encrypted registry credential values and metadata.

## What Is Not Backed Up

Database backups do not include:

- Docker images, containers, volumes, or container filesystems;
- live process state inside `orch-server` or `orch-agent`;
- centralized logs, Prometheus TSDB data, or Grafana dashboards;
- `.env` files, systemd drop-ins, TLS keys, JWT signing secrets, or agent bootstrap tokens;
- the secret encryption key used by the local envelope provider;
- external registry state.

Back up configuration and keys separately using your normal secret-management process.

## Secret Key Requirements

Restored encrypted secrets and registry credentials can only be decrypted with the same envelope key material used when they were written.

For the local envelope provider, preserve `ORCH_SECRET_KEY` or the equivalent configured key. If the key is lost, restored secret and registry credential rows remain present but unusable; rotate/recreate those secrets after restore.

## Create A Backup

The backup script requires an explicit database URL and never defaults to local credentials:

```sh
scripts/backup-db.sh \
  --database-url "$DATABASE_URL" \
  --output backups/orch-$(date -u +%Y%m%dT%H%M%SZ).dump
```

The script uses:

```sh
pg_dump --format=custom --no-owner --no-privileges
```

Store backups in an encrypted location with restricted access.

## Restore Into A New Environment

1. Provision an empty PostgreSQL database.
2. Configure `ORCH_SECRET_KEY`, JWT signing secret, TLS materials, and agent registration token in the new environment.
3. Restore the dump:

   ```sh
   scripts/restore-db.sh \
     --database-url "$NEW_DATABASE_URL" \
     --input backups/orch-20260714T120000Z.dump \
     --yes
   ```

4. Start `orch-server` against the restored database.
5. Run any newer migrations if the binary expects schema changes newer than the backup.
6. Start agents and verify:

   ```sh
   orch node ls
   orch service ls
   orch events --limit 20
   orch audit --limit 20
   ```

## Restore After Accidental Service Deletion

Prefer the least destructive recovery path:

1. Stop additional automated changes if possible.
2. Restore the backup into a temporary database, not the live database.
3. Inspect the deleted service, service versions, routes, and deployment history in the temporary database.
4. Recreate the service from its saved spec or YAML.
5. Verify tasks converge and compare audit/event history.

If a point-in-time database recovery is available and the deletion affected many objects, restore to a new database at a timestamp before deletion, validate it, then cut over the server during a maintenance window.

## Restore After Database Loss

1. Stop `orch-server` to prevent writes to a partially restored database.
2. Provision a replacement PostgreSQL database.
3. Restore the latest known-good backup into the replacement database.
4. Reapply required runtime configuration and secret keys.
5. Start `orch-server`.
6. Restart agents or wait for heartbeat/token rotation.
7. Validate service desired state, task state, events, audit logs, and metrics.

## Destructive Restore

`scripts/restore-db.sh --clean` asks PostgreSQL to drop matching existing objects before recreating them from the backup.

```sh
scripts/restore-db.sh \
  --database-url "$DATABASE_URL" \
  --input backups/orch-20260714T120000Z.dump \
  --clean \
  --yes
```

Warning: `--clean` is destructive. Use it only against an empty replacement database or during an approved maintenance window after confirming the backup is valid.

## Plain SQL Dumps

The restore script can apply a plain SQL dump with `psql`:

```sh
scripts/restore-db.sh \
  --database-url "$DATABASE_URL" \
  --input backups/orch.sql \
  --yes
```

`--clean` is supported only for custom-format dumps because `pg_restore` controls object cleanup for that format.
