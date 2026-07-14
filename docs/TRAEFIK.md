# Traefik Integration

`orch` can expose an optional Traefik HTTP provider endpoint for services that define HTTP routes. Core scheduling, reconciliation, service discovery, and Docker runtime behavior do not require Traefik.

## Service Routes

Add `routes` to a service YAML file:

```yaml
name: traefik-demo
image: nginx:1.27-alpine
replicas: 2
ports:
  - container: 80
    public: 0
routes:
  - host: demo.localhost
    pathPrefix: /
    port: 80
    tls: false
resources:
  cpu: 100m
  memory: 128Mi
restart:
  policy: always
placement:
  labels:
    role: worker
```

Route fields:

- `host`: HTTP host rule for Traefik.
- `pathPrefix`: path prefix rule, such as `/` or `/api`.
- `port`: service container port to route to.
- `tls`: when `true`, the generated router includes Traefik TLS config.

## Dynamic Config Endpoint

Traefik can poll:

```text
GET /v1/integrations/traefik/config
```

The response is Traefik dynamic config JSON. It includes only active services with route definitions and only TCP endpoints from tasks whose desired state is running and whose actual state is `running` or `healthy`. Unhealthy, stopped, unassigned, and non-matching-port tasks are omitted, so Traefik sees updates on its next poll after tasks start, stop, or become unhealthy.

## Local Development

Start the local orchestrator and Traefik:

```sh
docker compose --profile local-orch --profile traefik up --build
```

Deploy the demo service:

```sh
go run ./cmd/orch --server http://localhost:8080 deploy deployments/examples/traefik-demo.yaml
```

Traefik polls the control plane with:

```sh
--providers.http.endpoint=http://orch-server:8080/v1/integrations/traefik/config
--providers.http.pollInterval=2s
```

Then open:

```sh
curl -H 'Host: demo.localhost' http://localhost:8082/
```

The Traefik dashboard is available at `http://localhost:8088/` in this local compose profile.
