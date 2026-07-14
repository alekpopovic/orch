# 82. Cluster upgrade framework

```text
Design and implement a cluster upgrade framework.

Context:
The orchestrator itself has server, agent, database schema, and CLI versions. Production upgrades need compatibility checks.

Task:
Add version compatibility and upgrade documentation.

Requirements:
- Add version endpoint:
  - GET /v1/version
- Agents report version in registration and heartbeat.
- Server records agent version.
- Add compatibility rules:
  - minimum supported agent version
  - maximum tested agent version
  - database schema version
- Add CLI:
  - orch version
  - orch cluster check-upgrade
- Add docs/UPGRADES.md with:
  - server upgrade
  - agent rolling upgrade
  - database migration
  - rollback procedure
- Add tests for incompatible agent rejection/warning.
- Add startup check for database schema version.

At the end:
- Run go test ./...
```
