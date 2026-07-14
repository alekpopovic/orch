# 46. Audit logging

```text
Implement audit logging for user and agent actions.

Context:
Production systems need an immutable audit trail for operational actions.

Task:
Add audit log model and persistence.

Audit events should include:
- actor type: user, agent, system
- actor ID
- action
- target type
- target ID
- request ID
- source IP if available
- timestamp
- outcome: success/failure
- metadata with redaction

Capture:
- service create/update/delete
- scale
- rollout
- rollback
- node drain/uncordon
- secret create/delete
- registry credential create/delete
- agent registration
- agent token rotation

Requirements:
- Store audit logs separately from operational events.
- Never store secret plaintext.
- Add API for admin audit search.
- Add CLI:
  - orch audit
- Add tests.
- Update docs/SECURITY.md and docs/OPERATIONS.md.
```
