# 28. MVP release preparation

```text
Prepare the repository for an MVP release.

MVP acceptance criteria:
- orch-server starts cleanly
- orch-agent registers with server
- CLI can deploy a YAML service
- reconciler creates tasks for desired replicas
- scheduler assigns tasks to ready nodes
- agent starts Docker containers
- service can be scaled up/down
- logs can be viewed
- events are persisted
- service can be deleted
- tests pass
- local docker-compose demo works
- README quickstart is accurate

Tasks:
1. Run the full test suite.
2. Run lint.
3. Review README quickstart.
4. Fix broken flows.
5. Add missing tests for critical MVP paths.
6. Produce RELEASE_NOTES.md for v0.1.0.

Do not add large new features.
Focus on making the MVP reliable and demonstrable.
```
