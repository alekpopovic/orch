# 14. Rolling update v1

```text
Implement rolling update for services.

API:
POST /v1/services/{id}/rollout

Request:
{
  "image": "ghcr.io/example/api:1.0.1",
  "maxUnavailable": 1,
  "maxSurge": 1
}

Behavior:
- Create a new service version.
- Gradually create new-version tasks.
- Wait for new tasks to become healthy.
- Stop old-version tasks.
- Continue until all running tasks are on new version.
- Mark rollout successful.
- If a new task fails health checks, pause rollout and mark failed.
- Emit events for each rollout step.

Requirements:
- Implement as a rollout controller loop, not a blocking API request.
- API request should start rollout and return rollout ID.
- Add GET endpoint for rollout status.
- Add CLI:
  orch rollout api --image ghcr.io/example/api:1.0.1
  orch rollout status api
- Add tests for:
  - successful rollout
  - failed new task
  - max unavailable respected
  - max surge respected
  - idempotent resume after controller restart
- Document rollout state machine.
```
