# 62. API versioning strategy

```text
Design and implement API versioning rules.

Context:
The orchestrator exposes /v1 APIs. As the system matures, API compatibility must be explicit.

Task:
Create an API versioning strategy and enforce it in code.

Deliverables:
- docs/API_VERSIONING.md
- middleware or router structure for versioned APIs
- compatibility tests for selected endpoints

Requirements:
- Keep existing /v1 behavior stable.
- Define what counts as:
  - backward-compatible change
  - breaking change
  - deprecated field
  - removed endpoint
- Add deprecation metadata support.
- Add response headers for deprecated endpoints if needed.
- Add tests for v1 routing.
- Add docs explaining how future /v2 APIs should be introduced.
- Do not create /v2 yet unless required for structure.

At the end:
- Run go test ./...
```
