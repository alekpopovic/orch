# Reliability

## Idempotency Rules

Every controller and agent operation should be safe to retry after a timeout, process restart, or lost response.

- API create/update handlers validate input before mutating state. Mutating handlers should return the existing in-progress object when the requested operation is already underway.
- Reconciler passes must derive work from current service and task state. A second pass must not create extra tasks, repeat stop events, or inflate metrics for tasks that already have a stop/remove desired state.
- Scheduler assignment must use optimistic concurrency on the task row. If another scheduler assigned the task first, the retry should reload state rather than assign a different node blindly.
- Agent task execution must treat Docker operations as idempotent. Existing task containers are reused, stopped containers are safe to stop again, and missing containers are safe to remove again.
- Docker-created containers must carry `orch.managed=true` plus service, task, node, and version labels. Cleanup only targets managed containers for the current node.
- Rollout controllers must compute progress from task versions and terminal states. Repeated passes must not create more new-version tasks than `replicas + maxSurge`, stop more old tasks than allowed by `maxUnavailable`, or emit status-change events when the status did not change.
- Service deletion is soft by default. Repeated delete requests keep the service in `deleting`; final cleanup happens only after all service tasks report `removed`.

## Current Audit Notes

- The memory control plane ignores terminal tasks when reconciling service replicas, so retries after failed or removed tasks create replacements instead of counting dead tasks as active.
- The reconciler treats already-stopped tasks as no-ops and only records stop metrics/events when it actually changes desired task state.
- The rollout status helper emits events only on real status transitions.
- The Docker runtime wrapper already handles create conflicts by looking up the existing managed task container and treats stop/remove of already stopped or missing containers as success.

## Retry Guidance

Prefer polling current state with bounded timeouts over fixed sleeps. Tests and scripts should wait for observable state such as task status, service status, or health endpoints.

When adding a new mutating operation, add a table-driven test that calls the operation twice with the same input and verifies:

- no duplicate database rows;
- no duplicate containers;
- no misleading events or metrics;
- no unrelated task or service is selected for stop/remove;
- all timestamps remain UTC.
