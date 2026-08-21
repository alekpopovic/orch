# Service Discovery

Optional internal DNS resolves `<service>.<namespace>.svc.orch` and `<service>.svc.orch` to node addresses for healthy/running endpoints. Set `ORCH_DNS_ADDR` on `orch-server` (for example `0.0.0.0:53`) to enable UDP DNS. `ORCH_DNS_TTL` configures TTL and defaults to 30 seconds. The component tracks `dns_queries_total` and `dns_errors_total`.

MVP service discovery also exposes running task endpoints over the control-plane API and CLI. DNS intentionally does not implement full Kubernetes compatibility.

## API

```text
GET /v1/services/{id}/endpoints
GET /v1/discovery/services
GET /v1/discovery/services/{name}
```

By default, responses include tasks whose desired status is `running` and actual status is `running` or `healthy`.

Use `include_unhealthy=true` to also include `unhealthy` tasks:

```text
GET /v1/discovery/services/api?include_unhealthy=true
```

Each endpoint includes:

- service name;
- task ID;
- node ID;
- node advertise address;
- public host port;
- container port;
- protocol;
- health status;
- service version.

## CLI

```sh
orch endpoints api
orch endpoints api --include-unhealthy
orch endpoints api --output json
```

The CLI accepts a service name or ID, resolves it through the service API, and prints the current endpoint set.

## Notes

- Task-assigned ports are preferred over service template ports.
- Services with no published/container ports produce no endpoints.
- Deleted and deleting services are omitted from the all-services discovery endpoint.
- DNS and reverse-proxy integration are separate features.
