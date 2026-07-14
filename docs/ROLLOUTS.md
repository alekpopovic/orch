# Rollouts

Rollouts are asynchronous deployments between service versions. The API request starts the rollout and returns a deployment ID; the rollout controller advances it later.

## API And CLI

```sh
orch rollout api --image ghcr.io/example/api:1.0.1
orch rollout api --image ghcr.io/example/api:1.0.1 --max-unavailable 0 --max-surge 2
orch rollout status api
orch rollback api
```

```sh
curl -X POST http://localhost:8080/v1/services/{service_id}/rollout \
  -H 'Content-Type: application/json' \
  -d '{"image": "ghcr.io/example/api:1.0.1", "maxUnavailable": 1, "maxSurge": 1}'

curl http://localhost:8080/v1/services/{service_id}/rollout
curl http://localhost:8080/v1/rollouts/{rollout_id}
curl -X POST http://localhost:8080/v1/services/{service_id}/rollback
```

`maxUnavailable` and `maxSurge` must not both be zero.

## Operation Locking

Rollout, rollback, scale, and delete are guarded by a service-level operation lock:

- A second rollout conflicts while a different rollout is active.
- Repeating the same active rollout returns the existing deployment.
- Rollback conflicts while a rollout is active; forced rollback is not exposed yet.
- Scale conflicts while rollout or rollback is active.
- Delete wins and cancels active deployments before moving the service to `deleting`.

Conflicts are returned as `409 conflict` with `operation already in progress`.

## Deployment Statuses

- `pending`: accepted but not yet processed by the controller.
- `running`: creating target-version tasks and stopping old-version tasks.
- `succeeded`: all active tasks are on the target version and enough target tasks are healthy.
- `failed`: a target-version task failed.
- `paused`: reserved for future operator-controlled pause behavior.
- `rolling_back`: rollback deployment is active.
- `rolled_back`: rollback completed.

## Rolling Update Flow

For each active deployment, the controller:

1. Loads the service and all service tasks.
2. Computes rollout state from task versions and statuses.
3. Fails the rollout if any target-version task is `failed`.
4. Moves `pending` deployments to `running`.
5. Creates target-version tasks while `active < replicas + maxSurge`.
6. Waits for target-version tasks to become `healthy`.
7. Stops old-version tasks only while `maxUnavailable` remains respected.
8. Marks the deployment terminal when old active tasks are gone and target healthy count meets replicas.

The controller only creates tasks and stop directives. Scheduling and Docker execution remain separate.

## Rollback Flow

Rollback:

1. Finds the latest previous successful service version. Version 1 is treated as the initial baseline.
2. Restores the service spec to that version.
3. Creates a rollback deployment from current version to previous version.
4. Lets the rollout controller converge tasks through the same safety gates.

Failed rollout history is preserved.

If no previous successful version exists, rollback returns invalid state.

## Safety And Idempotency

The controller recomputes progress from current task state every pass. Restarting the controller does not create duplicate target-version tasks beyond the surge limit.

Status-change events are emitted only when the deployment status actually changes. Repeated rollback requests return the active rollback deployment when one is already in flight.

## Current Limitations

- No manual pause/resume endpoint yet.
- No automatic rollback on failed rollout.
- No forced rollback over an active rollout endpoint yet.
- No progress deadline.
- No transaction retry loop for serialization conflicts or deadlocks yet.
