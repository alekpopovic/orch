# Rollouts

Rollouts are asynchronous. `POST /v1/services/{id}/rollout` records a new service version and creates a deployment row, then returns the rollout ID immediately. The rollout controller loop advances the deployment in later passes.

## State Machine

- `pending`: rollout has been accepted but not yet processed by the controller.
- `running`: controller is creating new-version tasks and stopping old-version tasks.
- `succeeded`: all active tasks for the service are on the target version and enough target-version tasks are healthy.
- `failed`: at least one target-version task failed health checks or runtime startup.
- `paused`: reserved for operator-controlled pause behavior.
- `rolling_back`: rollback has selected a previous successful service version and is reconciling tasks back to that version.
- `rolled_back`: rollback completed; active tasks are back on the selected previous version.

## Rolling Update Rules

For each running deployment, the controller:

1. Loads the service, deployment, and all service tasks.
2. Fails the rollout if any target-version task is failed.
3. Creates target-version tasks while respecting `replicas + maxSurge`.
4. Waits for target-version tasks to report `healthy`.
5. Stops old-version tasks only when `maxUnavailable` remains respected.
6. Marks the rollout `succeeded` when no old-version tasks remain active and target-version healthy tasks meet the replica count.

Task execution remains the agent's responsibility. The rollout controller only creates desired tasks and marks old tasks desired-stopped; the scheduler places pending tasks, and agents reconcile Docker state.

## Idempotency

The controller derives every decision from persisted service, deployment, and task state. If it restarts, it resumes from the current tasks without creating duplicate target-version tasks beyond the allowed surge window.

Rollback requests are also idempotent while a rollback is in flight. Repeating `POST /v1/services/{id}/rollback` returns the active rollback deployment instead of creating a duplicate deployment.

## Rollback

Rollback preserves audit history. Failed rollout deployments remain in the deployment table and event stream.

When a rollback starts, the control plane:

1. Finds the latest successful service version before the current version. The initial service version is considered a successful baseline.
2. Restores the service spec to that version.
3. Creates a rollback deployment from the current version to the selected previous version.
4. Emits rollback events.
5. Lets the rollout controller stop failed/current-version tasks only after previous-version tasks satisfy health and availability rules.

If no previous successful version exists, the API returns an invalid-state error.

## API

```sh
curl -X POST http://localhost:8080/v1/services/{service_id}/rollout \
  -H 'Content-Type: application/json' \
  -d '{"image": "ghcr.io/example/api:1.0.1", "maxUnavailable": 1, "maxSurge": 1}'

curl -X POST http://localhost:8080/v1/services/{service_id}/rollback
curl http://localhost:8080/v1/services/{service_id}/rollout
curl http://localhost:8080/v1/rollouts/{rollout_id}
```

## CLI

```sh
orch rollout api --image ghcr.io/example/api:1.0.1
orch rollout api --image ghcr.io/example/api:1.0.1 --max-unavailable 0 --max-surge 2
orch rollout status api
orch rollback api
```
