# Architecture

`orch` is a Go-based container orchestrator for multi-node Docker deployments. The first runtime target is Docker Engine API, with runtime access isolated behind interfaces so the control plane can later support containerd.

## System Overview

The system has three primary binaries:

- `orch-server`: control plane API server that owns desired state, scheduling decisions, deployment workflows, authentication, and persistence.
- `orch-agent`: worker node agent that reports node state, runs local reconciliation against assigned tasks, performs health checks, and streams node events.
- `orch`: Cobra-based CLI for operators and automation.

PostgreSQL stores durable desired state, observed state, deployment history, events, and rollout metadata. Prometheus metrics will expose control plane, agent, scheduler, reconciler, runtime, and health check behavior as the implementation grows.

## Core Components

- API server: validates requests, enforces authorization, writes desired state, and exposes stable request/response contracts.
- Scheduler: deterministically places tasks on nodes based on inputs such as node health, capacity, constraints, and existing assignments.
- Reconciler: continuously compares desired state with observed state and issues idempotent runtime operations until the node converges.
- Store: owns PostgreSQL access through `sqlc` or `pgx` and hides persistence details from domain packages.
- Docker runtime: wraps Docker Engine API operations for image, network, volume, container, and health-related behavior.
- Healthchecker: performs HTTP and TCP checks with timeouts, context cancellation, and deterministic test doubles.
- Rollout manager: coordinates rolling updates, rollback, health gates, and deployment progress.
- Events, logs, and metrics: provide operator visibility and machine-readable orchestration history.

## Control Flow

1. A user or automation calls `orch` or the control plane API.
2. `orch-server` validates and persists desired state in PostgreSQL.
3. The scheduler computes deterministic task placements.
4. Agents receive or poll their assigned desired state.
5. Each agent reconciler compares desired task state with local Docker state.
6. The Docker runtime adapter performs idempotent Docker Engine API operations.
7. Health checks, events, logs, and metrics update observed state and deployment progress.
8. The rollout manager advances, pauses, or rolls back deployments based on health and policy.

## Design Principles

- Keep domain logic independent from Docker, PostgreSQL, clocks, and network probes.
- Make scheduler and reconciler behavior deterministic and unit-testable.
- Treat runtime operations as convergent, not one-shot commands.
- Keep APIs explicit, validated, and backward-compatible.
- Support graceful shutdown through context cancellation in every long-running loop.
- Store all timestamps in UTC.
- Protect secrets through encryption at rest and redaction everywhere else.
