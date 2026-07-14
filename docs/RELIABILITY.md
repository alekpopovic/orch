# Reliability

`orch` is designed around deterministic reconciliation. Controllers and agents should converge from current state instead of assuming one-shot commands always succeed.

## Reliability Boundary Today

The current `orch-server` binary uses in-memory control-plane state. That means server process restart loses services, tasks, deployments, events, node credentials, and node status. PostgreSQL migrations and store code exist, but durable server wiring is roadmap.

The agent and Docker runtime contain recovery logic for local process restarts and Docker state drift, but durable cluster recovery requires PostgreSQL-backed control-plane state.

## Idempotency Rules

- API mutations validate input before changing state.
- Repeated delete requests keep the service in `deleting`; hard delete is not default.
- Reconciler passes derive work from current service/task state.
- Scheduler assignment uses optimistic concurrency.
- Scheduler emits events only for persisted assignments.
- Assignment conflicts and missing tasks are benign races; the next pass recomputes.
- Agent status reports cannot resurrect tasks after stop/remove or terminal state.
- Duplicate terminal status reports are idempotent and do not emit duplicate events.
- Docker containers are identified by orchestrator labels and task ID.
- Docker stop/remove of already-stopped or missing containers is success.
- Rollout passes recompute progress from tasks and deployment status.

## Failure Recovery

### Agent Process Restart

The agent re-registers, heartbeats, polls tasks, and searches Docker by managed labels. If it finds the task container, it reports the recovered container ID. No local durable cache is required for MVP.

### Server Process Restart

Current default behavior loses in-memory state. Use only for local development. Production requires wiring `orch-server` to PostgreSQL before relying on restart recovery.

### Docker Daemon Restart

Docker calls fail while unavailable. The agent does not create replacement containers if it cannot list managed containers. When Docker returns, the agent reconciles by labels and task desired state.

### Node Goes Offline

Graceful shutdown sends a heartbeat with `shutdown=true`, marking the node `offline`. Abrupt loss is not detected by heartbeat expiry yet.

### Node Returns

Registration with the same stable node name reuses the node ID in the current control plane and marks offline/unknown nodes ready.

### Manual `docker rm`

If a task container was manually removed, the agent confirms no managed container exists for the task and recreates it for the same task ID if desired state is still running.

### Container Exits Non-Zero

The agent reports the task `failed` with a failure reason. Replacement is a reconciler decision based on restart policy.

### Image Pull Fails

The agent reports `failed`; the event stream records `task.failed` with error severity.

### Port Already Allocated

Docker create/start failure is reported as `failed` with the runtime error as failure reason.

### Database Temporarily Unavailable

Store operations return errors. Controllers should retry from fresh state on the next loop. Full database retry/backoff policy is roadmap.

### Transaction Boundaries

Durable control-plane code must wrap multi-write operations in `store.WithTx`. The callback receives a store interface, not a database-specific transaction type.

Critical transaction boundaries:

- service create: service row, initial service version row, and `service.created` event;
- scale: service update, reconciliation side effects, and `service.scaled` event;
- scheduler assignment: task assignment and `task.assigned` event;
- agent status report: task status update and task status/failure/health event;
- rollout request: service version update, deployment row, and rollout event;
- rollback request: service version restore, rollback deployment row, and rollback event;
- service deletion: service `deleting` mark, task stop directives, and deletion-started event.

PostgreSQL-backed transactions use row-level locking for task assignment preflight and still finish with an atomic `UPDATE ... actual_status = 'pending'` guard. Concurrent scheduler attempts must result in at most one persisted assignment and one assignment event.

The in-memory control plane currently protects these same multi-write flows with a single mutex. When `orch-server` is wired to PostgreSQL, equivalent API and controller flows must use `store.WithTx` rather than issuing separate store calls.

### Rollout Interrupted Mid-Way

The rollout controller recomputes state from deployment and task records. It respects max surge and max unavailable on each pass.

## Task Lifecycle Guardrails

Task actual status is reported by agents, but control-plane desired status has priority.

If desired status is `stopped` or `removed`, stale agent reports such as `running` are rejected. Agents may still report stopping/terminal statuses so cleanup can complete.

Once a task is `removed`, only a duplicate `removed` report is accepted.

## Testing Expectations

Every reliability-sensitive change should add tests for:

- retrying the same API/controller operation;
- process restart or stale state replay;
- duplicate event prevention;
- no duplicate containers;
- no duplicate task assignments;
- UTC timestamps;
- context cancellation for loops.

## Roadmap

- Wire server to PostgreSQL.
- Add heartbeat expiry and node-failure rebalancing.
- Add real leader election.
- Add automatic transaction retries for serialization conflicts and deadlocks.
- Add progress deadlines for rollouts.
- Add persistent event export.
