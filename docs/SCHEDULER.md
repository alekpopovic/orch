# Scheduler

The scheduler assigns pending tasks to suitable ready nodes. It is deterministic, unit-testable, and has no Docker dependency.

## Responsibility

The scheduler:

- reads pending tasks;
- reads ready nodes;
- reads already-placed non-terminal tasks;
- reads service specs for resource requests and placement constraints;
- writes task assignments with optimistic concurrency;
- emits `task.assigned` events for assignments it actually persisted.

The scheduler does not create tasks, stop tasks, start containers, or modify services. Those are reconciler, rollout, agent, and API responsibilities.

## Inputs

- Tasks with `actual_status=pending`.
- Nodes with `status=ready`.
- Existing tasks assigned to each ready node.
- Service specs for pending and existing tasks.

## Filtering

A node is eligible for a pending task only if:

- node status is `ready`;
- service placement constraints match node labels;
- free CPU and memory are greater than or equal to the service resource requests.

Draining and offline nodes are excluded because they are not `ready`.

Free resources are calculated as:

```text
node.allocatable - requests(non-terminal tasks already on node) - requests(tasks planned earlier in this pass)
```

See [RESOURCES.md](RESOURCES.md) for CPU and memory normalization.

## Scoring

Eligible nodes are scored in this order:

1. More free memory after placement.
2. Fewer active tasks for the same service.
3. Fewer active tasks overall.
4. Lexicographically smallest node ID.

Pending tasks are processed in stable task ID order. Nodes are processed in stable node ID order. This makes repeated scheduling passes deterministic for the same input.

## Persistence And Races

Assignments are persisted through `AssignTask(ctx, taskID, nodeID, expectedUpdatedAt)`. Returned scheduler assignments include:

- task ID;
- selected node ID;
- assigned timestamp;
- expected task `updated_at` guard used for the claim.

If the store returns `ErrConflict` or `ErrNotFound`, the scheduler treats that as a benign race and skips that planned assignment. It returns and emits events only for assignments that were actually persisted. The next pass recomputes from fresh state.

PostgreSQL assignment uses row-level locking with `FOR UPDATE SKIP LOCKED` inside the transaction and then updates only rows that still match `actual_status='pending'` and the expected `updated_at` value. This makes a pending task claim atomic when two scheduler loops accidentally run at the same time. One loop can claim the task; later or concurrent loops observe a conflict or skipped locked row and do not emit duplicate assignment events.

There is no HA leader election yet. Correctness depends on task-level atomic claims, deterministic planning, and benign conflict handling rather than a single active scheduler process.

## Scheduler Metrics

- `scheduler_scheduling_attempts_total`: scheduler passes started.
- `scheduler_scheduling_failures_total`: passes that failed before completion.
- `scheduler_tasks_claimed_total`: pending tasks successfully claimed.
- `scheduler_assignment_conflicts_total`: stale, locked, or already-claimed assignment attempts.

## Current Limitations

- No preemption.
- No anti-affinity beyond the simple spreading score.
- No taints/tolerations.
- No heartbeat-expiry integration yet.
- No volume, network topology, or port-conflict-aware scheduling.
