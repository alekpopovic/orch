# Scheduler

Scheduler v1 assigns pending tasks to ready worker nodes. It is deterministic and has no Docker dependency; the scheduler only reads desired/current state from the store and writes task assignments plus events.

## Inputs

- pending tasks with `actual_status=pending`.
- ready nodes with labels, status, capacity, and allocatable resources.
- running tasks already placed on each node.
- service specs for pending and running tasks, including resource requests and placement constraints.

## Filtering

For each pending task, the scheduler considers nodes in stable `node_id` order and filters out nodes that are not suitable:

- node status must be `ready`.
- draining nodes are excluded because they are not `ready`.
- service placement constraints must match node labels.
- free allocatable CPU and memory must be greater than or equal to the service resource requests.

Free resources are calculated from node allocatable resources minus the resource requests of running tasks already on that node, plus any assignments planned earlier in the same scheduler pass.

## Scoring

Eligible nodes are scored in this order:

1. Prefer more free memory after placing the task.
2. Prefer fewer running tasks for the same service.
3. Prefer fewer total running tasks.
4. Prefer the lexicographically smallest node ID as the final deterministic tie-breaker.

Pending tasks are processed in stable `task_id` order. After each assignment, the in-memory scheduling state is updated so later tasks see the planned CPU, memory, and task counts.

## Persistence

`RunOnce` performs one scheduling pass:

1. Load pending tasks.
2. Load ready nodes.
3. Load running tasks for those nodes.
4. Load service specs needed for task resource and placement decisions.
5. Plan assignments.
6. Persist each assignment with optimistic concurrency using the task `updated_at`.
7. Emit a `task.assigned` event.

The store remains the source of truth. If another scheduler or control-plane operation updates a task first, the store returns a conflict and the next scheduler pass can retry from fresh state.
