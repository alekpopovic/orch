# Autoscaling

`orch` includes an autoscaling controller MVP for service replica counts. The MVP is CPU-based and intentionally uses a metrics provider abstraction so controller tests do not require Prometheus, Docker, or network access.

## Service YAML

```yaml
name: api
image: ghcr.io/example/api:1.0.0
replicas: 2
resources:
  cpu: 500m
  memory: 512Mi
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilization: 70
  cooldown: 60s
```

Fields:

- `enabled`: turns autoscaling decisions on for the service.
- `minReplicas`: lower bound for desired replicas.
- `maxReplicas`: upper bound for desired replicas.
- `targetCPUUtilization`: target average CPU utilization percentage.
- `cooldown`: minimum time between autoscaler-initiated scale operations.
- `stabilizationWindow`: reserved for conservative scale-down behavior; persisted in the spec for compatibility.

## Controller Behavior

The autoscaler:

1. Lists active services with autoscaling enabled.
2. Skips services with an active rollout or rollback.
3. Reads CPU utilization from the configured metrics provider.
4. Computes `ceil(currentReplicas * observedCPU / targetCPU)`.
5. Clamps the recommendation to `minReplicas..maxReplicas`.
6. Respects cooldown.
7. Calls the same `ScaleService` control-plane path used by the API and CLI.

Scale decisions emit `autoscaler.scaled`, `autoscaler.skipped`, or `autoscaler.error` events when an event store is provided.

## Metrics

Server metrics expose:

- `autoscaler_decisions_total{decision=...}`
- `autoscaler_errors_total`
- `autoscaler_recommendation_replicas{service_id=...}`

## Current Limitations

- The built-in MVP provides the controller and fake metrics provider; a production Prometheus metrics provider is still future work.
- Cooldown state is in controller memory until durable autoscaling status is added.
- Scale-down stabilization is represented in the API shape but not yet enforced beyond cooldown.
