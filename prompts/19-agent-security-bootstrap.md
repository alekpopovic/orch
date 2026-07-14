# 19. Agent security bootstrap

```text
Improve agent authentication.

Current state:
Agent uses a static bootstrap token.

Implement:
- Agent registration token.
- Server issues short-lived agent credential after successful registration.
- Agent uses credential for heartbeat and task polling.
- Add token rotation support.
- Add server-side revocation field for nodes.

Design for future mTLS:
- Create interfaces so token auth can later be replaced or combined with mTLS.
- Document future mTLS design in docs/SECURITY.md.

Requirements:
- Never store raw tokens in DB; store hashed tokens.
- Never print tokens in logs.
- Add tests for token issuance, validation, expiration, and revocation.
```
