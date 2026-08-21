# Database Migrations

Migration files are ordered pairs in `migrations/`. Applied versions are recorded in `schema_migrations`; status discovery is deterministic and can be run repeatedly without changing schema state.

```sh
orch-server migrate status --database-url "$DATABASE_URL"
orch-server migrate up --database-url "$DATABASE_URL"
orch-server migrate down --database-url "$DATABASE_URL" --allow-down
```

`up` creates the version table when needed, takes a cluster-wide PostgreSQL advisory lock, and applies every pending migration in its own transaction. A second invocation skips recorded versions. `down` is rejected unless `--allow-down` is supplied and the latest migration has a down file.

Before applying a production migration, take and test a backup, read both directions, verify compatibility with one still-running server version, and test it against a production-sized copy. Prefer additive columns/tables and bounded backfills. Never rewrite a committed migration. Down migrations that discard operator data require an explicit maintenance decision; rolling a binary back against the additive schema is generally safer.

Set `ORCH_SCHEMA_CHECK=true` for production server startup. The server refuses schema versions outside the range advertised by `/v1/version`; v0.3.0 accepts 15–16 and expects 16. The opt-in remains necessary while the default development server uses its in-memory control plane.

