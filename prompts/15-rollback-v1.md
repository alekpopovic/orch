# 15. Rollback v1

```text
Implement rollback support.

API:
POST /v1/services/{id}/rollback

Behavior:
- Find previous successful service version.
- Start a rollout back to that version.
- Preserve audit trail.
- Emit rollback events.

Requirements:
- Do not delete failed rollout history.
- Add CLI:
  orch rollback api
- Add tests:
  - rollback after failed rollout
  - rollback when no previous version exists
  - rollback idempotency
- Update docs/ROLLOUTS.md.
```
