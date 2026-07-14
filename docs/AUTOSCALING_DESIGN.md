# Autoscaling Design

This document proposes autoscaling for `orch` services without changing the current deterministic scheduler and reconciler contract.

## Goals

- Scale service replicas from observed workload signals.
- Support CPU, memory, queue depth, and custom metrics over time.
- Keep scale decisions deterministic, auditable, and safe during rollouts.
- Reuse the existing `ScaleService` path so reconciliation and scheduler behavior stay consistent.
- Make the MVP testable with fake metrics and fake clocks.

## Non-Goals

- No predictive scaling in the MVP.
- No per-container vertical autoscaling.
- No direct task mutation outside the control plane service scale path.
- No dependency on Prometheus for unit tests.
- No multi-tenant quota integration until namespace quotas exist.

## Metrics Sources

Autoscaling should use a `MetricsProvider` interface:

```go
type MetricsProvider interface {
    ServiceCPUUtilization(ctx context.Context, serviceID types.ServiceID) (float64, error)
    ServiceMemoryUtilization(ctx context.Context, serviceID types.ServiceID) (float64, error)
    CustomMetric(ctx context.Context, serviceID types.ServiceID, name string) (float64, error)
}
```

Provider options:

- **Fake provider** for deterministic tests.
- **Prometheus provider** for production once queries are stable.
- **Queue provider** later for queue depth integrations such as Redis, SQS-compatible queues, or custom HTTP exporters.

Metric samples should include timestamp, source, value, and confidence. Missing or stale metrics must not cause blind scale-down.

## Scaling Targets

The service spec should add optional autoscaling:

```yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilization: 70
  cooldown: 60s
  stabilizationWindow: 300s
```

Initial target types:

- `targetCPUUtilization`: average CPU usage divided by requested CPU.
- `targetMemoryUtilization`: future target; memory scale-down requires conservative stabilization.
- `customMetrics`: future metric list with target values and direction.

## Min/Max Replicas

- `minReplicas` prevents scaling below required availability.
- `maxReplicas` prevents runaway scaling and scheduler pressure.
- `minReplicas <= maxReplicas` is required.
- Manual scale may either disable autoscaling or be treated as an operator override; MVP should keep autoscaling enabled but record a drift/override event when manual scale moves outside min/max.

## Cooldown Periods

Cooldown avoids repeated scale actions from noisy metrics:

- `cooldown` applies after every autoscaler-initiated scale.
- Scale-up may use a shorter cooldown than scale-down in the future.
- Cooldown state should be stored with the service or autoscaling status so controller restart does not immediately repeat a decision.

## Stabilization Windows

Stabilization windows smooth recent recommendations:

- Scale-up can use the highest recent recommendation.
- Scale-down should use the highest recommendation within the window to avoid flapping.
- Missing metrics should freeze scale-down and may permit scale-up only when policy explicitly allows.

## Scale Policies

The decision loop:

1. List active services with autoscaling enabled.
2. Skip services in active rollout unless the rollout strategy explicitly allows autoscaling.
3. Fetch metric samples.
4. Compute recommended replicas:
   - CPU example: `ceil(currentReplicas * currentUtilization / targetUtilization)`.
5. Clamp to `minReplicas..maxReplicas`.
6. Apply stabilization and cooldown.
7. Call the existing control plane `ScaleService`.
8. Emit event and metrics.

Scale-up policies:

- Default max step: double current replicas or add 4, whichever is larger.
- Never exceed `maxReplicas`.
- Prefer fast response to sustained high CPU.

Scale-down policies:

- Default max step: remove 25% of replicas per decision.
- Never go below `minReplicas`.
- Require healthy metrics throughout the stabilization window.

## Rollout Interaction

Default MVP behavior should skip autoscaling while a rollout or rollback is active for the service. This keeps rollout availability math stable and avoids surprise changes to target task counts.

Future behavior may allow scale-up during rollout if:

- the deployment controller stores the desired replica count at rollout start;
- surge/unavailable math is recalculated safely;
- events clearly attribute scale decisions during rollout.

## Node Capacity Interaction

Autoscaling recommends desired replicas; the scheduler remains responsible for placement.

Failure modes:

- If capacity is insufficient, `ScaleService` may increase desired replicas and the scheduler records pending tasks.
- Autoscaler should emit a warning event when recommendations exceed schedulable capacity.
- Future quota/capacity estimators can clamp recommendations before scale.

## Failure Modes

- **Metrics unavailable:** do not scale down; emit warning.
- **Metrics stale:** do not scale down; optionally scale up only with explicit policy.
- **Control plane conflict:** retry after next interval.
- **Active rollout:** skip and record reason.
- **Scheduler unable to place tasks:** keep desired state, emit event, rely on operator/node changes.
- **Controller restart:** recover cooldown and last recommendation from persisted status.
- **Clock skew:** store all timestamps in UTC and inject clocks in tests.

## API Proposal

Public service response should include:

```json
{
  "autoscaling": {
    "enabled": true,
    "min_replicas": 2,
    "max_replicas": 10,
    "target_cpu_utilization": 70,
    "cooldown": "60s",
    "stabilization_window": "300s"
  },
  "autoscaling_status": {
    "last_scale_at": "2026-07-14T12:00:00Z",
    "last_recommendation": 4,
    "last_observed_cpu_utilization": 82.5,
    "condition": "AbleToScale"
  }
}
```

Possible endpoints:

- `GET /v1/services/{id}/autoscaling`
- `PUT /v1/services/{id}/autoscaling`
- `DELETE /v1/services/{id}/autoscaling`

The MVP may initially accept autoscaling config only through service create/update YAML and expose status through service responses.

## YAML Proposal

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
  stabilizationWindow: 300s
```

## Events And Metrics

Events:

- `autoscaler.scaled`
- `autoscaler.skipped`
- `autoscaler.metrics_unavailable`
- `autoscaler.capacity_limited`

Metrics:

- `autoscaler_decisions_total{decision="scale_up|scale_down|skip"}`
- `autoscaler_recommendation_replicas`
- `autoscaler_observed_cpu_utilization`
- `autoscaler_errors_total`

## Test Strategy

Unit tests:

- recommendation math;
- min/max clamping;
- cooldown enforcement with fake clock;
- stabilization windows;
- missing/stale metrics;
- active rollout skip;
- control plane scale path integration with fakes.

Integration tests:

- fake metrics provider scales a memory control plane service up and down;
- scheduler creates pending tasks for new desired replicas;
- events and metrics are emitted.

The design keeps autoscaling as a controller that writes desired replicas through existing APIs, so scheduler and reconciler determinism remains unchanged.
