# Reliability

## Idempotency Rules

Every controller and agent operation should be safe to retry after a timeout, process restart, or lost response.

- API create/update handlers validate input before mutating state. Mutating handlers should return the existing in-progress object when the requested operation is already underway.
- Reconciler passes must derive work from current service and task state. A second pass must not create extra tasks, repeat stop events, or inflate metrics for tasks that already have a stop/remove desired state.
- Scheduler assignment must use optimistic concurrency on the task row. If another scheduler assigned the task first, the retry should reload state rather than assign a different node blindly.
- A scheduler pass returns and emits events only for assignments it actually persisted. Assignment conflicts and missing tasks are treated as benign races; the next pass recomputes from fresh state.
- Agent task status reports must not resurrect a task after the control plane has requested stop/remove or after the task is terminal. Exact duplicate terminal reports are idempotent and do not emit another event.
- Agent task execution must treat Docker operations as idempotent. Existing task containers are reused, stopped containers are safe to stop again, and missing containers are safe to remove again.
- Docker-created containers must carry `orch.managed=true` plus service, task, node, and version labels. Cleanup only targets managed containers for the current node.
- Rollout controllers must compute progress from task versions and terminal states. Repeated passes must not create more new-version tasks than `replicas + maxSurge`, stop more old tasks than allowed by `maxUnavailable`, or emit status-change events when the status did not change.
- Service deletion is soft by default. Repeated delete requests keep the service in `deleting`; final cleanup happens only after all service tasks report `removed`.

## Current Audit Notes

- The memory control plane ignores terminal tasks when reconciling service replicas, so retries after failed or removed tasks create replacements instead of counting dead tasks as active.
- The control plane rejects stale active agent reports for tasks whose desired state is stopped or removed.
- The reconciler treats already-stopped tasks as no-ops and only records stop metrics/events when it actually changes desired task state.
- PostgreSQL task assignment and stop operations return the existing task when a retry observes the requested state already applied.
- The rollout status helper emits events only on real status transitions.
- The Docker runtime wrapper already handles create conflicts by looking up the existing managed task container and treats stop/remove of already stopped or missing containers as success.

## Failure Mode Behavior

### Agent Process Restarts

Current behavior: the agent re-registers by node name, gets the existing node ID, heartbeats, and polls assigned tasks.

Unsafe behavior: if the server has a task without a container ID but Docker still has a managed container for that task, the agent could leave server state stale.

MVP fix: before creating a container, the agent searches Docker by managed task labels. If it finds a running task container, it reports `running` with the recovered container ID.

### Server Process Restarts

Current behavior: the local `orch-server` binary currently uses the in-memory control plane, so service/task state is lost on restart. PostgreSQL store and migrations exist, but the server wiring is not fully persistent yet.

Unsafe behavior: agents can keep local containers running while the restarted control plane has no desired-state record for them.

MVP rule: use this mode for local development only. Production wiring must use PostgreSQL-backed stores before relying on restart recovery.

### Docker Daemon Restarts

Current behavior: Docker calls fail while the daemon is unavailable.

Unsafe behavior: treating an inspect error as “container missing” can lead to duplicate create attempts after Docker returns.

MVP fix: when inspect fails, the agent checks managed task labels. If Docker cannot list containers, the agent returns the error and does not create a replacement until Docker is reachable.

### Node Goes Offline

Current behavior: graceful shutdown heartbeats mark the node `offline`. Unexpected loss is not detected by a lease/timeout controller yet.

Unsafe behavior: without heartbeat expiry, an abruptly dead node can remain `ready`.

MVP rule: graceful shutdown is handled; heartbeat-expiry based offline marking is future work and should be deterministic before HA scheduling is enabled.

### Node Returns After Being Offline

Current behavior: registering with the same node name reuses the existing node ID and marks `offline` or `unknown` nodes `ready`.

Unsafe behavior: returning as a new node would orphan assigned tasks.

MVP fix: node registration remains keyed by hostname for local MVP recovery and reuses existing identity.

### Task Is Manually Deleted With `docker rm`

Current behavior: the agent may have a stale container ID in server state.

Unsafe behavior: failing permanently on the stale ID would leave the task stuck.

MVP fix: the agent verifies by managed labels. If the container is truly absent and Docker is reachable, it pulls/creates/starts a replacement for the same task.

### Container Exits Non-Zero

Current behavior: the agent inspects assigned containers.

Unsafe behavior: blindly restarting an exited non-zero container hides the failure from the control plane.

MVP fix: non-zero exited containers are reported as `failed` with an error event. Replacement is left to server-side reconciliation policy.

### Image Pull Fails

Current behavior: the agent reports task failure.

Unsafe behavior: failure events were previously generic status events.

MVP fix: task failures emit `task.failed` with `error` severity and preserve the failure reason.

### Port Is Already Allocated

Current behavior: Docker create/start returns an error.

Unsafe behavior: without a clear failed state, the task can look merely in progress.

MVP fix: create/start errors are reported as `failed`; the failure reason is stored and an error event is emitted.

### Database Temporarily Unavailable

Current behavior: store operations return errors to API handlers/controllers.

Unsafe behavior: if a write partially succeeds and the response is lost, clients may retry and hit conflicts or duplicates depending on the operation.

MVP rule: mutations use optimistic concurrency where implemented, and controllers retry from fresh state on the next loop. Full database retry/backoff policy remains future work.

### Rollout Interrupted Mid-Way

Current behavior: the rollout controller derives progress from deployment status and task versions.

Unsafe behavior: repeated passes could create too many surge tasks or emit duplicate status-change events.

MVP fix: rollout progress is recomputed from current tasks; max surge/unavailable limits are recalculated every pass, and status events are emitted only when the status changes.

## Retry Guidance

Prefer polling current state with bounded timeouts over fixed sleeps. Tests and scripts should wait for observable state such as task status, service status, or health endpoints.

When adding a new mutating operation, add a table-driven test that calls the operation twice with the same input and verifies:

- no duplicate database rows;
- no duplicate containers;
- no misleading events or metrics;
- no unrelated task or service is selected for stop/remove;
- all timestamps remain UTC.
