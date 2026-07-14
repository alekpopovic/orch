# State Machines

This document is the stable internal contract for `orch` lifecycle state. API handlers, the in-memory control plane, the scheduler, the reconciler, the rollout controller, agents, and stores must use the lifecycle helpers in `pkg/types`.

All state transitions are explicit. Invalid transitions return a domain error wrapping `types.ErrInvalidTransition`; API and store boundaries wrap that as an invalid-state request.

## Service Lifecycle

States:

- `active`: service desired state is reconciled.
- `deleting`: deletion has been requested and tasks are converging to removal.
- `deleted`: terminal state; the service is retained for history but no longer reconciled.

Allowed transitions:

- `active -> deleting`
- `deleting -> deleted`
- same-state updates are idempotent

Forbidden transitions:

- `deleted -> active`
- `deleted -> deleting`
- `active -> deleted`

`deleted` is terminal. Recovery from `deleted` is intentionally not supported; create a new service instead.

## Task Lifecycle

Task desired state is owned by the server. Task actual state is reported by the agent.

Actual states:

- `pending`: created by the reconciler or rollout controller and waiting for scheduling.
- `assigned`: scheduler selected a node.
- `pulling`: agent is pulling the image.
- `created`: container exists but is not yet running.
- `starting`: reserved for runtimes with an explicit start phase.
- `running`: container process is running.
- `healthy`: health checks pass.
- `unhealthy`: health checks fail but policy may allow recovery or replacement.
- `stopping`: reserved for explicit stop progress.
- `stopped`: terminal runtime stop state.
- `removed`: terminal cleanup state.
- `failed`: terminal failure state.

Allowed actual transitions:

- `pending -> assigned|stopped|removed|failed`
- `assigned -> pulling|created|running|healthy|unhealthy|stopping|stopped|removed|failed`
- `pulling -> created|running|stopping|stopped|removed|failed`
- `created -> starting|running|stopping|stopped|removed|failed`
- `starting -> running|stopping|stopped|removed|failed`
- `running -> healthy|unhealthy|stopping|stopped|removed|failed`
- `healthy -> running|unhealthy|stopping|stopped|removed|failed`
- `unhealthy -> running|healthy|stopping|stopped|removed|failed`
- `stopping -> stopped|removed|failed`
- `failed -> removed`
- `stopped -> removed`
- same-state updates are idempotent

Terminal actual states:

- `stopped`
- `removed`
- `failed`

Terminal states must not be mutated except for documented cleanup recovery to `removed`. A duplicate terminal report is idempotent and must not emit duplicate state-change events.

Desired state transitions:

- `running -> stopped`
- `running -> removed`
- `stopped -> removed`
- same-state updates are idempotent

Retryable behavior is policy-driven. `unhealthy` may recover to `running` or `healthy`; `failed` is terminal for that task, but the reconciler may create a replacement task when the service restart policy allows it.

Task conditions are additive metadata and do not change lifecycle validity. `node_lost` records that the server marked the owning node offline after heartbeat expiry. Stateless `node_lost` tasks are failed/removed for replacement; stateful `node_lost` tasks remain assigned for manual recovery.

## Node Lifecycle

States:

- `unknown`: persisted node exists but its current health is not known.
- `ready`: node accepts new task assignments.
- `draining`: node keeps existing work but receives no new scheduler placements.
- `offline`: node is gracefully shut down or unreachable.

Allowed transitions:

- `unknown -> ready|offline`
- `ready -> draining|offline`
- `draining -> ready|offline`
- `offline -> ready`
- same-state updates are idempotent

`offline -> ready` is the documented recovery path for a returning node that re-registers, heartbeats after a healed partition, or is explicitly uncordoned.

## Deployment And Rollout Lifecycle

States:

- `pending`: rollout request was accepted but the controller has not advanced it.
- `running`: rolling update is active.
- `paused`: reserved for future operator-controlled pause behavior.
- `succeeded`: terminal successful rollout.
- `failed`: terminal failed rollout.
- `rolling_back`: rollback rollout is active.
- `rolled_back`: terminal successful rollback.

Allowed transitions:

- `pending -> running|rolling_back|succeeded|failed|rolled_back`
- `running -> succeeded|failed|paused`
- `paused -> running|failed`
- `rolling_back -> rolled_back|failed|paused`
- same-state updates are idempotent

Terminal deployment states:

- `succeeded`
- `failed`
- `rolled_back`

Terminal deployments are immutable. Rollback creates a new `rolling_back` deployment rather than mutating a failed or succeeded deployment.

## Agent Task Execution Lifecycle

Agents may report only:

- `pulling`
- `created`
- `running`
- `healthy`
- `unhealthy`
- `failed`
- `stopped`
- `removed`

Agent status reports are accepted only when they are valid task actual-state transitions and do not conflict with server desired state.

Special rules:

- A task with desired `stopped` or `removed` accepts only cleanup reports: `stopped`, `removed`, or `failed`.
- A task already `removed` accepts only a duplicate `removed` report.
- A terminal task accepts only a duplicate terminal report, except `failed` and `stopped` may move to `removed` during cleanup.
- Healthcheck `unhealthy` may be converted to `failed` when the service restart policy allows replacement.

## Compatibility Rules

- Do not rename or remove lifecycle states without a versioned API migration.
- Additive states require updates to `pkg/types`, this document, API docs, transition tests, and every switch over lifecycle states.
- The scheduler, reconciler, rollout controller, agent status path, and store implementations must call the shared lifecycle helpers instead of duplicating transition rules.
