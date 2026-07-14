# 35. Basic service discovery

```text
Implement basic service discovery for MVP.

Context:
The orchestrator currently runs containers across Docker hosts. Services need a way to discover running task endpoints.

Task:
Add a service discovery endpoint and CLI support.

API:
- GET /v1/services/{id}/endpoints
- GET /v1/discovery/services
- GET /v1/discovery/services/{name}

Endpoint data:
- service name
- task ID
- node ID
- node address
- public host port
- container port
- protocol
- health status
- service version

Requirements:
- Only return running and healthy tasks by default.
- Add query option include_unhealthy=true.
- Add CLI:
  - orch endpoints <service>
- Add tests for endpoint filtering.
- Add docs/SERVICE_DISCOVERY.md.
- Keep it simple; do not implement DNS yet.
```
