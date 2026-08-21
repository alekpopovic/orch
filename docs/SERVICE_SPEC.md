# Service Spec

`orch deploy` accepts a YAML service spec and validates it before sending it to the server. The server repeats validation through the domain `ServiceSpec` model and must not trust CLI input.

The canonical machine-readable schema is `schemas/service.schema.json`.

The namespace is request metadata, not a field in the deployment YAML. Select it with `orch --namespace <name> deploy ...`, `ORCH_NAMESPACE`, CLI config, or the API `X-Orch-Namespace` header.

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

## Security Context

`securityContext` controls container runtime hardening:

```yaml
securityContext:
  user: "1000:1000"
  readOnlyRootFilesystem: true
  capDrop:
    - NET_RAW
  capAdd:
    - NET_BIND_SERVICE
  hostNetwork: false
  hostPID: false
  hostPathMounts:
    - hostPath: /var/lib/orch-volumes/api
      containerPath: /data
      readOnly: true
```

By default, `orch` rejects privileged containers, host networking, host PID, host path mounts outside configured prefixes, and capability additions outside the cluster allowlist. If `capDrop` is omitted, the Docker runtime drops `NET_RAW` by default.

## Autoscaling

`autoscaling` enables CPU-based replica autoscaling through the autoscaler controller:

```yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilization: 70
  cooldown: 60s
```

The controller updates desired replicas through the normal service scale path, clamps decisions to min/max replicas, respects cooldown, and skips services with an active rollout.

## Validation

Validate locally without creating a service:

```sh
orch validate deployments/examples/http-api.yaml
```

Healthcheck `path` must start with a single `/` and must not be a full URL. Healthcheck `scheme` defaults to `http` and may be `http` or `https`.

The CLI rejects unknown YAML fields, invalid ports, invalid resources, invalid healthcheck settings, invalid placement labels, invalid routes, invalid security context paths or capabilities, and malformed secret references. The server additionally runs centralized admission for create, rollout, and scale. Depending on cluster policy it can require requests/limits and healthchecks, restrict registries and host paths, block `latest`, and cap replicas or public ports. Rejections include stable rule IDs in `error.details.violations`.
