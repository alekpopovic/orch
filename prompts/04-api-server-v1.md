# 04. API server v1

```text
Implement the first version of the control-plane REST API.

Endpoints:

Health:
- GET /healthz
- GET /readyz

Nodes:
- GET /v1/nodes
- GET /v1/nodes/{id}
- POST /v1/nodes/{id}/drain
- POST /v1/nodes/{id}/uncordon

Services:
- POST /v1/services
- GET /v1/services
- GET /v1/services/{id}
- DELETE /v1/services/{id}
- POST /v1/services/{id}/scale
- POST /v1/services/{id}/rollout
- POST /v1/services/{id}/rollback

Tasks:
- GET /v1/tasks
- GET /v1/tasks/{id}

Events:
- GET /v1/events

Requirements:
- Use net/http or chi.
- Add request/response DTOs.
- Validate all inputs.
- Return structured JSON errors.
- Add request ID middleware.
- Add structured access logs.
- Add panic recovery middleware.
- Add timeouts.
- Keep handlers thin.
- Business logic should live in service/usecase packages.
- Add unit tests for handlers.
- Add API examples to docs/API.md.

Do not implement auth yet, but structure the middleware chain so auth can be added later.
```
