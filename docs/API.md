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

User-facing `/v1/*` endpoints require `Authorization: Bearer <jwt>` when `ORCH_JWT_SECRET` is configured. Agent endpoints use the separate bootstrap-token flow documented below.

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

Resource values are normalized as CPU millicores and memory bytes. The API also accepts string inputs such as `"500m"`, `"2.5"`, `"512Mi"`, and `"1Gi"` for resource fields.

Delete is safe by default: it marks the service `deleting`, asks agents to stop and remove task containers, and only moves the service to `deleted` after all tasks report `removed`.

Scale and rollout operations:

```sh
curl -X POST http://localhost:8080/v1/services/{service_id}/scale \
  -H 'Content-Type: application/json' \
  -d '{"replicas": 4}'

curl -X POST http://localhost:8080/v1/services/{service_id}/rollout \
  -H 'Content-Type: application/json' \
  -d '{"image": "nginx:1.28", "maxUnavailable": 1, "maxSurge": 1}'

curl http://localhost:8080/v1/services/{service_id}/rollout
curl http://localhost:8080/v1/rollouts/{rollout_id}

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

Agent registration requires `Authorization: Bearer <ORCH_AGENT_REGISTRATION_TOKEN>`. Registration returns a short-lived agent credential used for heartbeat, task polling, and task status updates.

```sh
curl -H 'Authorization: Bearer <agent-credential>' \
  'http://localhost:8080/v1/agent/tasks?node_id={node_id}'

curl -X POST http://localhost:8080/v1/agent/tasks/{task_id}/status \
  -H 'Authorization: Bearer <agent-credential>' \
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
curl 'http://localhost:8080/v1/events?service_id={service_id}&limit=50'
curl 'http://localhost:8080/v1/events?task_id={task_id}&type=task.status'
curl 'http://localhost:8080/v1/events?node_id={node_id}&severity=warning'
curl 'http://localhost:8080/v1/events?since=2026-06-11T10:00:00Z'
```

## Logs

```sh
curl 'http://localhost:8080/v1/logs?service_id={service_id}&tail=100'
curl 'http://localhost:8080/v1/logs?service_id={service_id}&follow=true'
curl 'http://localhost:8080/v1/logs?service_id={service_id}&task_id={task_id}'
```

The MVP response is newline-delimited text proxied from a worker agent.
