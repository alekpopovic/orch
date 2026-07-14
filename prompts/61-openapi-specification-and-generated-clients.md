# 61. OpenAPI specification and generated clients

```text
Create and maintain an OpenAPI specification for the orchestrator API.

Context:
The repository contains orch-server REST API, orch CLI, agents, scheduler, reconciler, events, logs, rollouts, secrets, registry credentials, and node operations.

Task:
Add a formal OpenAPI v3 specification for the public user-facing API.

Deliverables:
- api/openapi.yaml
- docs/API.md updated to reference the OpenAPI spec
- generated or hand-written Go API client package under pkg/client
- CLI updated to use the shared client package where practical

Requirements:
- Include endpoints for:
  - health
  - nodes
  - services
  - tasks
  - events
  - logs
  - rollouts
  - secrets
  - registry credentials
  - audit logs
- Include request/response schemas.
- Include structured error schema.
- Include auth scheme.
- Add CI check to validate OpenAPI syntax.
- Add tests proving API handlers match documented response shapes where practical.
- Do not expose internal agent endpoints in the public OpenAPI spec unless clearly separated.

At the end:
- Run go test ./...
- Run OpenAPI validation.
- Update README with API client usage example.
```
