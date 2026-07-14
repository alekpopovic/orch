# Reconciler

The service reconciler keeps service replica intent aligned with task desired state. It creates pending tasks and stop directives; it does not schedule tasks and does not call Docker.

## Responsibility

The reconciler:

- lists services;
- lists tasks for each service;
- creates missing pending tasks for active services;
- stops excess active tasks;
- replaces failed restartable tasks by creating new pending tasks;
- stops tasks for deleted or missing services;
- advances soft service deletion to `deleted` after all tasks report `removed`;
- emits events for task creation, stop decisions, and deletion completion;
- records metrics for runs, errors, duration, created tasks, and stopped tasks.

## Loop

`Run(ctx)` calls `ReconcileOnce(ctx)` immediately, then repeats on a configurable interval. The loop respects context cancellation.

A leader-lock abstraction exists, but the current implementation uses a no-op lock. Do not run multiple production reconcilers against the same durable store until real locking is implemented.

## Active Service Reconciliation

For each active service:

1. Load all service tasks.
2. Separate current-version non-terminal tasks from outdated tasks.
3. Stop outdated non-terminal tasks.
4. Count current-version non-terminal tasks.
5. Count failed current-version tasks only when restart policy does not allow replacement.
6. If effective count is below replicas, create missing pending tasks.
7. If current non-terminal count is above replicas, stop extra tasks in deterministic stop order.

The scheduler later assigns pending tasks. Agents later run or stop containers.

## Stop Order

When reducing active tasks, the reconciler prefers stopping less-disruptive tasks first:

1. `pending`
2. `assigned`
3. `pulling`, `created`, `starting`
4. `unhealthy`
5. `running`, `healthy`
6. `stopping`

Within the same priority, task IDs are ordered deterministically.

## Restart Policy

For `always`, `on_failure`, and the empty default, failed tasks do not count toward desired replicas. The reconciler creates replacements.

For `never`, failed tasks count toward effective replicas. This preserves a stopped/failure state for operator inspection.

## Service Deletion

Delete is soft:

1. API marks the service `deleting`.
2. The reconciler sets non-removed service tasks to desired `stopped`.
3. Agents stop/remove containers and report `removed`.
4. When every service task is `removed`, the reconciler marks the service `deleted`.

Repeated delete and reconcile passes are idempotent.

## Current Limitations

- No durable leader election yet.
- Individual create/stop/status-plus-event writes use `store.WithTx` when the store supports transactions; a full reconciliation pass is still intentionally recomputed step by step.
- No controller for abrupt node heartbeat expiry yet.
- The default `orch-server` binary currently uses the in-memory control plane rather than this store-backed reconciler loop.
