# 60. Release hardening v0.2.0

```text
Prepare the repository for v0.2.0 production-hardening release.

Acceptance criteria:
- All tests pass.
- Lint passes.
- API docs match implementation.
- Service spec docs match validation.
- No known secret leaks in logs.
- Reconciler is idempotent.
- Scheduler is concurrency-safe.
- Agent recovers after restart.
- Node offline detection works.
- Service deletion is safe.
- Rollout/rollback state machine is documented.
- Local docker-compose demo works.
- Production deployment docs exist.

Tasks:
1. Run full test suite.
2. Run lint.
3. Run security-sensitive grep checks for tokens/secrets logging.
4. Run local demo if possible.
5. Fix blockers.
6. Update CHANGELOG.md.
7. Create RELEASE_NOTES.md for v0.2.0.

Do not add new features unless required to satisfy acceptance criteria.
```
