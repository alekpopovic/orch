# 37. Secrets MVP

```text
Implement MVP secrets support.

Context:
Services should reference secrets without storing plaintext secret values in service YAML or logs.

YAML example:

env:
  DATABASE_URL:
    secretRef: prod/database-url
  NODE_ENV:
    value: production

Task:
Add secrets model and API.

API:
- POST /v1/secrets
- GET /v1/secrets
- GET /v1/secrets/{name}
- DELETE /v1/secrets/{name}

Requirements:
- Store encrypted secret values at rest.
- Use envelope-style abstraction even if MVP uses a local master key.
- Never return secret plaintext from GET endpoints.
- Never log secret values.
- Agent should receive plaintext only when needed to start a task.
- Redact secret values in errors and events.
- Add config for encryption key.
- Add tests for:
  - create secret
  - retrieve metadata
  - inject secret into task env
  - redaction in logs/errors
  - wrong encryption key behavior
- Add docs/SECRETS.md.

Security note:
Do not implement external Vault integration yet, but design the interface so it can be added later.
```
