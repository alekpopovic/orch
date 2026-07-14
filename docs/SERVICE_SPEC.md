# Service Spec

`orch deploy` accepts a YAML service spec and validates it before sending it to the server. The server repeats validation through the domain `ServiceSpec` model and must not trust CLI input.

The canonical machine-readable schema is `schemas/service.schema.json`.

## Required Fields

- `name`: stable service name.
- `image`: container image reference without whitespace.

## Common Fields

- `replicas`: desired replica count, `>= 0`; defaulting happens server-side.
- `imagePullSecret`: registry credential ID created with `orch registry-credential` API/CLI flows.
- `stateful`: marks workloads that should not be auto-replaced after node loss.
- `resources.cpu`: CPU request such as `100m`, `0.5`, or `1`.
- `resources.memory`: memory request such as `128Mi` or `1Gi`.
- `restart.policy`: `always`, `never`, or `on_failure`.

## Ports And Routes

`ports[].container` is required and must be `1..65535`. `ports[].public` is optional; `0` means dynamically assigned.

Routes require `host`, `pathPrefix` beginning with `/`, and `port` matching the service container port.

## Env And Secrets

`env` values can be literal strings or secret references:

```yaml
env:
  NODE_ENV: production
  DATABASE_URL:
    secretRef: prod/database-url
```

An env entry cannot specify both `value` and `secretRef`.

## Validation

Validate locally without creating a service:

```sh
orch validate deployments/examples/http-api.yaml
```

The CLI rejects unknown YAML fields, invalid ports, invalid resources, invalid healthcheck settings, invalid placement labels, invalid routes, and malformed secret references.
