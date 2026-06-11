# API

The control-plane API uses JSON request and response bodies. Errors use a stable envelope:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "invalid state: image is required",
    "request_id": "9f95d3c2-9a78-4b11-a11f-8991d45db5e1"
  }
}
```

Clients may send `X-Request-ID`; otherwise the server generates one.

## Health

```sh
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

## Nodes

```sh
curl http://localhost:8080/v1/nodes
curl http://localhost:8080/v1/nodes/00000000-0000-4000-8000-000000000001
curl -X POST http://localhost:8080/v1/nodes/00000000-0000-4000-8000-000000000001/drain
curl -X POST http://localhost:8080/v1/nodes/00000000-0000-4000-8000-000000000001/uncordon
```

## Services

Create a service:

```sh
curl -X POST http://localhost:8080/v1/services \
  -H 'Content-Type: application/json' \
  -d '{
    "spec": {
      "name": "web",
      "image": "nginx:1.27",
      "replicas": 2,
      "env": {"APP_ENV": "local"},
      "secret_refs": [{"name": "registry-creds", "key": "password"}],
      "ports": [{"protocol": "tcp", "container_port": 8080, "published_port": 18080}],
      "resource_requirements": {
        "requests": {"cpu": 100, "memory": 134217728},
        "limits": {"cpu": 500, "memory": 536870912}
      },
      "healthcheck": {
        "type": "http",
        "path": "/healthz",
        "port": 8080,
        "interval": 10000000000,
        "timeout": 2000000000,
        "healthy_threshold": 1,
        "unhealthy_threshold": 3
      },
      "restart_policy": {"condition": "on_failure", "max_attempts": 3},
      "placement_constraints": [{"key": "region", "operator": "equals", "value": "local"}]
    }
  }'
```

List, fetch, and delete:

```sh
curl http://localhost:8080/v1/services
curl http://localhost:8080/v1/services/{service_id}
curl -X DELETE http://localhost:8080/v1/services/{service_id}
```

Scale and rollout operations:

```sh
curl -X POST http://localhost:8080/v1/services/{service_id}/scale \
  -H 'Content-Type: application/json' \
  -d '{"replicas": 4}'

curl -X POST http://localhost:8080/v1/services/{service_id}/rollout \
  -H 'Content-Type: application/json' \
  -d '{"image": "nginx:1.28"}'

curl -X POST http://localhost:8080/v1/services/{service_id}/rollback
```

## Tasks

```sh
curl http://localhost:8080/v1/tasks
curl http://localhost:8080/v1/tasks?service_id={service_id}
curl http://localhost:8080/v1/tasks?node_id={node_id}
curl http://localhost:8080/v1/tasks?status=running
curl http://localhost:8080/v1/tasks/{task_id}
```

## Agent Tasks

Agent endpoints require `Authorization: Bearer <ORCH_BOOTSTRAP_TOKEN>` when a bootstrap token is configured.

```sh
curl -H 'Authorization: Bearer local-bootstrap-token' \
  'http://localhost:8080/v1/agent/tasks?node_id={node_id}'

curl -X POST http://localhost:8080/v1/agent/tasks/{task_id}/status \
  -H 'Authorization: Bearer local-bootstrap-token' \
  -H 'Content-Type: application/json' \
  -d '{
    "node_id": "{node_id}",
    "status": "running",
    "container_id": "container-id"
  }'
```

The task poll response includes each task plus service healthcheck metadata and ports so the agent can probe running containers.

Agents may report `pulling`, `created`, `running`, `healthy`, `unhealthy`, `failed`, `stopped`, and `removed`.

## Events

```sh
curl http://localhost:8080/v1/events
curl 'http://localhost:8080/v1/events?related_object_type=service&related_object_id={service_id}&limit=50'
```
