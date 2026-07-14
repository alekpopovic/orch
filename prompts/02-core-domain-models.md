# 02. Core domain models

```text
Design and implement the core domain model for the orchestrator.

Create Go types under pkg/types or internal/domain for:

Node:
- ID
- hostname
- advertise address
- labels
- capacity CPU/memory
- allocatable CPU/memory
- status: ready, draining, offline, unknown
- last heartbeat timestamp
- created/updated timestamps

Service:
- ID
- name
- image
- replicas
- env vars
- secret references
- ports
- resource requests/limits
- healthcheck
- restart policy
- placement constraints
- deployment version
- created/updated timestamps

Task:
- ID
- service ID
- node ID
- container ID
- desired status
- actual status
- image
- version
- restart count
- failure reason
- timestamps

Deployment/Rollout:
- ID
- service ID
- from version
- to version
- strategy
- status
- max unavailable
- max surge
- timestamps

Event:
- ID
- type
- severity
- source
- message
- related object type/id
- timestamp

Requirements:
- Use strong typed enums where appropriate.
- Add JSON tags for API serialization.
- Add validation methods for ServiceSpec and NodeSpec.
- Add table-driven unit tests for validation.
- Keep models free from database-specific annotations unless necessary.
- Document the lifecycle of Service -> Task -> Container in docs/ARCHITECTURE.md.
```
