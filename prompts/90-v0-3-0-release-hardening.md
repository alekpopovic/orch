# 90. v0.3.0 release hardening

```text
Prepare the repository for v0.3.0 release.

Expected v0.3.0 scope:
- OpenAPI spec
- API versioning
- admission controller
- namespaces/projects
- quotas
- GitOps MVP
- jobs MVP if implemented
- improved storage/networking design docs
- upgrade/migration safety
- stronger tests

Acceptance criteria:
- All tests pass.
- Lint passes.
- Migration status works.
- API docs match implementation.
- OpenAPI spec validates.
- Namespaces isolate resources.
- Quotas are enforced transaction-safely.
- Admission rejects unsafe specs.
- GitOps sync is tested.
- No secrets or credentials are logged.
- Release notes are accurate.

Tasks:
1. Run full test suite.
2. Run lint.
3. Run OpenAPI validation.
4. Run migration status tests.
5. Run security-sensitive grep checks.
6. Fix blockers only.
7. Update CHANGELOG.md.
8. Create RELEASE_NOTES.md for v0.3.0.

Do not add new features during release hardening.
```
