# API

`orch-server` exposes a JSON REST API for operators and agents. User-facing routes are under `/v1/*`; agent routes are under `/v1/agent/*`.

All responses use UTC timestamps.

## Errors

Errors use a stable envelope:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "invalid state: image is required",
    "request_id": "9f95d3c2-9a78-4b11-a11f-8991d45db5e1"
  }
}
```

Clients may send `X-Request-ID`. If absent, the server generates one and includes it in errors and access logs.

## Authentication

User API:

- If `ORCH_JWT_SECRET` is empty, user auth is disabled. This is the local-development default.
- If `ORCH_JWT_SECRET` is set, user-facing `/v1/*` endpoints require `Authorization: Bearer <jwt>`.
- Roles are `viewer`, `operator`, and `admin`.

Agent API:

- `POST /v1/agent/register` uses `Authorization: Bearer <ORCH_AGENT_REGISTRATION_TOKEN>`.
- Registration returns a short-lived agent credential.
- Heartbeat, task polling, and task status updates use the issued credential.

See [SECURITY.md](SECURITY.md).

## Health And Metrics

```sh
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/metrics
```

`readyz` currently reports readiness for the in-process server. It does not yet validate PostgreSQL connectivity because the shipped server is not wired to the PostgreSQL store.

## Nodes

```sh
curl http://localhost:8080/v1/nodes
curl http://localhost:8080/v1/nodes/{node_id}
curl -X POST http://localhost:8080/v1/nodes/{node_id}/drain
curl -X POST http://localhost:8080/v1/nodes/{node_id}/uncordon
```

Node operations:

- `drain` marks a node draining, excluding it from new scheduler placements.
- `uncordon` marks a node ready.
- Abrupt heartbeat expiry marks stale ready/draining nodes `offline` through the server node monitor.

## Services

Create:

```sh
curl -X POST http://localhost:8080/v1/services \
  -H 'Content-Type: application/json' \
  -d '{
    "spec": {
      "name": "web",
      "image": "nginx:1.27",
      "image_pull_secret": "ghcr-prod",
      "stateful": false,
      "replicas": 2,
      "env": {"APP_ENV": "local"},
      "secret_refs": [{"name": "prod/database-url", "env": "DATABASE_URL"}],
      "ports": [{"protocol": "tcp", "container_port": 8080, "published_port": 18080}],
      "routes": [{"host": "web.localhost", "path_prefix": "/", "port": 8080, "tls": false}],
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
      "placement_constraints": [{"key": "role", "operator": "equals", "value": "worker"}]
    }
  }'
```

Read and delete:

```sh
curl http://localhost:8080/v1/services
curl http://localhost:8080/v1/services/{service_id}
curl -X DELETE http://localhost:8080/v1/services/{service_id}
```

Delete is soft. It marks the service `deleting`, directs tasks to stop/remove, and marks the service `deleted` only after all tasks report `removed`.

Scale:

```sh
curl -X POST http://localhost:8080/v1/services/{service_id}/scale \
  -H 'Content-Type: application/json' \
  -d '{"replicas": 4}'
```

Rollout:

```sh
curl -X POST http://localhost:8080/v1/services/{service_id}/rollout \
  -H 'Content-Type: application/json' \
  -d '{"image": "nginx:1.28", "maxUnavailable": 1, "maxSurge": 1}'

curl http://localhost:8080/v1/services/{service_id}/rollout
curl http://localhost:8080/v1/rollouts/{rollout_id}
```

Rollback:

```sh
curl -X POST http://localhost:8080/v1/services/{service_id}/rollback
```

Concurrent service operations are guarded per service. Active rollout/rollback blocks scale, rollout, and rollback with `409 conflict` and a message containing `operation already in progress`. Repeating the same in-flight rollout with the same image and limits returns the existing deployment. Delete wins over rollout/rollback/scale by moving the service to `deleting` and cancelling active deployments.

Resource values are normalized to CPU millicores and memory bytes. The CLI deploy parser accepts strings such as `500m`, `2.5`, `512Mi`, and `1Gi`.

## Secrets

```sh
curl -X POST http://localhost:8080/v1/secrets \
  -H 'Content-Type: application/json' \
  -d '{"name":"prod/database-url","value":"postgres://user:pass@db/app"}'

curl http://localhost:8080/v1/secrets
curl http://localhost:8080/v1/secrets/prod%2Fdatabase-url
curl -X DELETE http://localhost:8080/v1/secrets/prod%2Fdatabase-url
```

Secret GET responses include metadata only and never return plaintext. Services can reference secrets with `secret_refs`, and agents receive decrypted values only in assigned task payloads. See [SECRETS.md](SECRETS.md).

## Registry Credentials

```sh
curl -X POST http://localhost:8080/v1/registry-credentials \
  -H 'Content-Type: application/json' \
  -d '{"id":"ghcr-prod","registry":"ghcr.io","username":"robot","password":"token"}'

curl http://localhost:8080/v1/registry-credentials
curl -X DELETE http://localhost:8080/v1/registry-credentials/ghcr-prod
```

Registry credential responses include metadata only. Services can reference credentials with `image_pull_secret`; agents receive decrypted pull auth only for assigned tasks. See [REGISTRIES.md](REGISTRIES.md).

## Service Discovery

```sh
curl http://localhost:8080/v1/services/{service_id}/endpoints
curl http://localhost:8080/v1/discovery/services
curl http://localhost:8080/v1/discovery/services/{service_name}
curl 'http://localhost:8080/v1/discovery/services/{service_name}?include_unhealthy=true'
```

Discovery endpoints include active services and assigned task ports. By default, only tasks whose desired state is `running` and whose actual state is `running` or `healthy` are returned.

## Traefik

```sh
curl http://localhost:8080/v1/integrations/traefik/config
```

The Traefik endpoint returns optional HTTP-provider dynamic config for services with `routes`. It omits unhealthy, stopped, unassigned, non-TCP, and non-matching-port task endpoints. See [TRAEFIK.md](TRAEFIK.md).

## Tasks

```sh
curl http://localhost:8080/v1/tasks
curl 'http://localhost:8080/v1/tasks?service_id={service_id}'
curl 'http://localhost:8080/v1/tasks?node_id={node_id}'
curl 'http://localhost:8080/v1/tasks?status=running'
curl http://localhost:8080/v1/tasks/{task_id}
```

Valid task statuses include:

- `pending`
- `assigned`
- `pulling`
- `created`
- `starting`
- `running`
- `healthy`
- `unhealthy`
- `stopping`
- `stopped`
- `removed`
- `failed`

Lifecycle transition rules are documented in [STATE_MACHINES.md](STATE_MACHINES.md).

Tasks can include `conditions`; `node_lost` means the server detected a stale node heartbeat. Stateless lost tasks are failed/removed so replacements can be created on ready nodes. Stateful lost tasks keep the condition for operator-directed recovery.

## Agent Registration And Heartbeat

Register:

```sh
curl -X POST http://localhost:8080/v1/agent/register \
  -H 'Authorization: Bearer <registration-token>' \
  -H 'Content-Type: application/json' \
  -d '{
    "node_name": "node-a",
    "advertise_address": "http://node-a:8081",
    "labels": {"role": "worker"},
    "capacity": {"cpu": 4000, "memory": 8589934592},
    "allocatable": {"cpu": 3500, "memory": 7516192768}
  }'
```

Heartbeat:

```sh
curl -X POST http://localhost:8080/v1/agent/heartbeat \
  -H 'Authorization: Bearer <agent-credential>' \
  -H 'Content-Type: application/json' \
  -d '{"node_id": "{node_id}"}'
```

Heartbeat responses may include a rotated credential.

## Agent Tasks

Poll assigned tasks:

```sh
curl -H 'Authorization: Bearer <agent-credential>' \
  'http://localhost:8080/v1/agent/tasks?node_id={node_id}'
```

Report task status:

```sh
curl -X POST http://localhost:8080/v1/agent/tasks/{task_id}/status \
  -H 'Authorization: Bearer <agent-credential>' \
  -H 'Content-Type: application/json' \
  -d '{
    "node_id": "{node_id}",
    "status": "running",
    "container_id": "container-id"
  }'
```

Agents may report:

- `pulling`
- `created`
- `running`
- `healthy`
- `unhealthy`
- `failed`
- `stopped`
- `removed`

The server rejects status reports from the wrong node and rejects stale active reports for tasks that are stopping, removed, or terminal.

## Events

```sh
curl http://localhost:8080/v1/events
curl 'http://localhost:8080/v1/events?service_id={service_id}&limit=50'
curl 'http://localhost:8080/v1/events?task_id={task_id}&type=task.failed'
curl 'http://localhost:8080/v1/events?node_id={node_id}&severity=warning'
curl 'http://localhost:8080/v1/events?since=2026-06-11T10:00:00Z'
```

Events support filtering by service, task, node, event type, severity, and `since`.

## Logs

```sh
curl 'http://localhost:8080/v1/logs?service_id={service_id}&tail=100'
curl 'http://localhost:8080/v1/logs?service_id={service_id}&follow=true'
curl 'http://localhost:8080/v1/logs?service_id={service_id}&task_id={task_id}'
```

Logs are proxied from the agent that owns the task. The MVP uses newline-delimited text/chunked streaming and does not persist logs centrally.
