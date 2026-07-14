# 49. Backup and restore runbook

```text
Create backup and restore support for orchestrator state.

Context:
The orchestrator stores desired state, actual state, events, secrets metadata, and rollout history in PostgreSQL.

Task:
Add operational backup/restore documentation and helper scripts.

Deliverables:
- scripts/backup-db.sh
- scripts/restore-db.sh
- docs/BACKUP_RESTORE.md

Requirements:
- Scripts must use pg_dump and psql/pg_restore where appropriate.
- Scripts must fail fast.
- Scripts must require explicit database URL.
- Document:
  - what is backed up
  - what is not backed up
  - how secrets encryption key affects restore
  - restore into new environment
  - restore after accidental service deletion
  - restore after DB loss
- Add safety warning before destructive restore.
```
