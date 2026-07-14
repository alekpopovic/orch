# HA Control Plane Design

`orch-server` currently runs as a single control-plane process. This document describes a safe path to multiple control-plane instances while preserving deterministic controllers and PostgreSQL-backed state.

## Goals

- Run more than one `orch-server` for API availability.
- Ensure each mutating background controller has at most one active leader.
- Use PostgreSQL first because it is already the durable system of record.
- Keep controller loops cancellation-aware and safe when leadership is lost.
- Avoid introducing etcd/Raft until the project needs stronger distributed coordination.

## Non-Goals

- No multi-region consensus in the MVP.
- No active-active scheduler without locks.
- No replacement for PostgreSQL durability.
- No automatic database failover design in this document.
- No distributed container networking changes.

## Active-Active vs Active-Passive

Recommended shape:

- **API layer:** active-active. Every server can serve authenticated read/write API traffic.
- **Controllers:** active-passive per controller name. Multiple servers may run, but only the lock holder executes a given controller loop.

This preserves API availability while avoiding duplicated scheduler, reconciler, rollout, node monitor, and autoscaler decisions.

## Controllers Requiring Leader Locks

- `scheduler`: assigns pending tasks to nodes and allocates ports.
- `reconciler`: creates/stops tasks to match service desired state.
- `rollout controller`: advances rollout and rollback state machines.
- `node monitor`: marks stale nodes offline.
- `autoscaler`: changes desired replica counts.

Controllers should use independent lock names so one server may lead scheduler while another leads autoscaler. This avoids one overloaded leader becoming a bottleneck for every controller.

## Database Locking Options

### PostgreSQL Advisory Locks

Recommended MVP:

- Use `pg_try_advisory_lock` or session-level `pg_advisory_lock` per controller.
- Derive lock keys from stable controller names.
- Hold a dedicated DB connection for each lock.
- Release on context cancellation or connection close.
- Stop the controller loop immediately if the lock is lost.

Pros:

- No new infrastructure.
- Simple operational story.
- Lock loss follows DB session semantics.

Cons:

- DB outage pauses controller leadership.
- Split-brain safety depends on PostgreSQL connection/session correctness.
- Requires careful connection handling; lock connections must not be returned to a pool while active.

### Row-Based Leases

Alternative:

- Store leases in a table with owner ID, heartbeat timestamp, and TTL.
- Renew periodically in transactions.

Pros:

- Easy to inspect.
- Can include owner metadata.

Cons:

- More code and time/clock sensitivity.
- Requires expiration handling and careful transaction isolation.

### etcd/Raft

Future option for larger clusters:

- Use etcd leases/elections.
- Move selected control-plane coordination out of PostgreSQL.

Pros:

- Purpose-built distributed coordination.
- Stronger primitives for watches and leases.

Cons:

- More infrastructure and operational burden.
- Premature for current architecture.

## Failure Modes

### Leader Dies

PostgreSQL closes the lock-holding session and releases advisory locks. Other server instances acquire leadership on the next attempt. Controllers must be idempotent because a previous leader may have completed work immediately before death.

### Lock Lost

If the lock connection fails or renewal detects loss, the controller loop must stop promptly and return. The wrapper should emit metrics/events and retry acquisition after backoff.

### Split Brain

Advisory locks prevent split brain while all instances coordinate through the same PostgreSQL primary. Split brain can still occur if operators accidentally point servers at different databases or an unsafe DB failover allows dual primaries.

Mitigations:

- Run with a single PostgreSQL primary.
- Include database identity in startup logs.
- Add cluster ID checks before production HA.

### Database Unavailable

API writes and controller locks fail. Existing running tasks continue on agents, but scheduling, rollout advancement, node offline detection, and autoscaling pause.

### Network Partition

An isolated server that loses DB connectivity cannot hold or acquire locks and must stop controllers. API readiness should fail when DB-backed state is required.

## Migration Path

1. Add a `LeaderElector` interface and no-op implementation for tests.
2. Add PostgreSQL advisory lock implementation.
3. Wrap each controller `Run` function with lock acquisition.
4. Add controller leader metrics and lock acquisition failure metrics.
5. Run multiple `orch-server` instances with only one controller leader per controller.
6. Add startup/operations docs for HA server replicas.

## Testing Strategy

Unit tests:

- one leader active for a lock name;
- second instance fails or waits;
- lock released on cancellation;
- wrapped controller stops when lock is lost;
- controller work remains idempotent after restart.

Integration tests:

- PostgreSQL advisory lock behavior with two connections;
- server shutdown releases locks;
- simulated lock connection failure stops the loop.

## MVP Recommendation

Use PostgreSQL advisory locks with one lock per controller. Keep the API active-active and controllers active-passive. Do not introduce etcd yet.

## Implemented MVP

The repository includes `internal/leadership`:

- `LeaderElector` and `Lease` interfaces;
- a local in-memory elector for deterministic unit tests;
- a PostgreSQL advisory-lock elector using `pg_try_advisory_lock`;
- `RunWithLeadership`, which runs a controller only while a lease is held and cancels the controller context when the lease is lost;
- Prometheus metrics for `controller_leader_status` and `controller_leader_acquisition_failures_total`.

Production wiring should use the PostgreSQL elector with one lock name per controller: `scheduler`, `reconciler`, `rollout`, `node-monitor`, and `autoscaler`.
