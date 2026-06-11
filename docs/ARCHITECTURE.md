# Architecture

`orch` is a Docker-first container orchestrator written in Go. It models services as desired state, tasks as per-replica work items, nodes as worker capacity, and Docker containers as local runtime artifacts managed by agents.

The current codebase is built to separate control-plane decisions from runtime side effects. Scheduling, reconciliation, rollout progression, validation, and resource accounting are unit-testable without Docker.

## Current Implementation Boundary

Implemented today:

- REST API with request IDs, structured JSON errors, access logs, recovery middleware, timeouts, optional JWT user auth, and token-based agent auth.
- In-memory control plane used by the default `orch-server` binary.
- PostgreSQL migrations and a PostgreSQL store implementation with optimistic concurrency, but not yet wired into `orch-server`.
- Docker Engine runtime package used by `orch-agent`.
- Scheduler, reconciler, rollout controller, health checks, events, logs, and Prometheus metrics.

Roadmap:

- Wire `orch-server` to PostgreSQL for durable production state.
- Add heartbeat-expiry node monitor.
- Add real HA leader election.
- Add mTLS node identity.
- Add containerd runtime support.

## Components

### orch-server

The server exposes operator and agent APIs. It validates requests, enforces optional JWT/RBAC for user-facing endpoints, validates agent credentials for agent endpoints, owns the active control-plane implementation, and runs the rollout controller loop.

In the current binary, the control plane is `internal/controlplane.MemoryService`. It is suitable for local development and tests, not for restart-safe production state.

### orch-agent

The agent represents one worker node. It detects local CPU and memory, registers with the server, heartbeats, polls assigned tasks, reconciles Docker containers, runs health checks, streams logs, and reports task status.

The agent does not schedule tasks and does not decide service replica count. It follows server desired state.

### CLI

`orch` is a Cobra CLI that calls the REST API. It supports table and JSON output, deploy YAML parsing, service operations, node operations, rollouts, events, and logs.

### Store

`internal/store` defines interfaces and a PostgreSQL implementation. Migrations create tables for nodes, services, service versions, tasks, deployments, and events.

The store uses UUID primary keys, UTC timestamps, indexes for common queries, optimistic concurrency through timestamps/versions, and domain errors such as `ErrNotFound`, `ErrConflict`, `ErrInvalidState`, and `ErrDuplicate`.

### Scheduler

The scheduler reads pending tasks, ready nodes, running tasks, and service specs. It computes deterministic assignments and persists them with optimistic concurrency. It does not call Docker.

### Reconciler

The reconciler keeps active services aligned with desired replica counts. It creates pending tasks, stops excess tasks, replaces failed restartable tasks, and finalizes soft deletion after tasks are removed. It does not schedule tasks or call Docker.

### Rollout Controller

The rollout controller advances deployments asynchronously. It creates target-version tasks subject to surge limits, waits for healthy target tasks, stops old-version tasks subject to availability limits, and marks deployments succeeded, failed, or rolled back.

### Runtime

`internal/docker` wraps the Docker Engine API behind a `Runtime` interface. All orchestrator-created containers carry labels identifying service, task, node, version, and `orch.managed=true`.

## Core Data Model

### Node

A node records stable identity, hostname, advertise address, labels, capacity, allocatable resources, status, heartbeat time, and agent credential metadata.

Node statuses:

- `ready`
- `draining`
- `offline`
- `unknown`

### Service

A service is desired state: name, image, replicas, environment, secret references, ports, resource requirements, healthcheck, restart policy, placement constraints, status, and deployment version.

Service statuses:

- `active`
- `deleting`
- `deleted`

### Task

A task is one service replica for one service version. It records service ID, node ID, container ID, desired status, actual status, image, version, restart count, failure reason, and timestamps.

Task desired state is controlled by the server. Task actual state is reported by agents.

### Deployment

A deployment records rollout or rollback progress between service versions. It stores strategy, status, max unavailable, max surge, and timestamps.

### Event

Events record important control-plane and agent decisions with type, severity, source, message, related object, and timestamp.

## Service To Container Lifecycle

1. An operator creates a service through the CLI or API.
2. The control plane stores service desired state.
3. The reconciler creates pending tasks when replica count is below desired.
4. The scheduler assigns pending tasks to ready nodes.
5. The agent polls assigned tasks for its node.
6. The agent pulls the image, creates a labeled Docker container, starts it, and reports task status after each step.
7. Health checks report `healthy` or `unhealthy`.
8. If a task fails and restart policy allows replacement, the reconciler creates a new pending task.
9. When a service is deleted, tasks move toward stopped/removed and the service becomes `deleted` only after all tasks report removed.

## Deployment Flow

Create:

1. `orch deploy service.yaml` parses and validates YAML.
2. API receives `POST /v1/services`.
3. Service is created as `active`, version `1`.
4. Reconciliation creates tasks.
5. Scheduling assigns tasks.
6. Agents create containers and report status.

Scale:

1. `orch scale api --replicas N` calls `POST /v1/services/{id}/scale`.
2. Service replica count changes.
3. Reconciler creates missing tasks or stops extra tasks.
4. Scheduler and agents converge actual state.

Rolling update:

1. `orch rollout api --image new:image` calls `POST /v1/services/{id}/rollout`.
2. Service image and deployment version advance.
3. Deployment starts as `pending`.
4. Rollout controller creates new-version tasks and stops old-version tasks according to limits.
5. Deployment succeeds only after old active tasks are gone and target-version healthy tasks meet replica count.

Rollback:

1. `orch rollback api` selects the previous successful service version.
2. Service spec is restored.
3. A rollback deployment runs through the same rollout controller.

## Time, Context, And Determinism

- All timestamps are UTC.
- Long-running loops accept context cancellation.
- Scheduler and reconciler logic are deterministic over sorted inputs.
- External side effects are behind interfaces.
- Docker operations are idempotent by task label and container ID.
