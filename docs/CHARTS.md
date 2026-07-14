# Charts

The docs site renders Mermaid charts directly from Markdown. This keeps diagrams reviewable in pull requests and versioned with the release notes.

## Control Plane Flow

```mermaid
flowchart LR
    operator["👤 Operator"] --> cli["🖥️ orch CLI"]
    cli --> api["⚡ API server"]
    api --> control["🧠 Control plane"]
    control --> scheduler["🧭 Scheduler"]
    control --> reconciler["🔁 Reconciler"]
    control --> rollout["🚀 Rollout controller"]
    scheduler --> tasks["📦 Desired tasks"]
    reconciler --> tasks
    rollout --> tasks
    tasks --> agent["🤖 Agent"]
    agent --> docker["🐳 Docker Engine"]
    api --> telemetry["📈 Metrics + events + audit"]
```

## v0.2.0 Release Mix

```mermaid
pie showData
    title Production hardening focus
    "Security + policy" : 24
    "Observability" : 18
    "Backup + operations" : 16
    "Autoscaling" : 15
    "HA control plane" : 14
    "Load + chaos testing" : 13
```

## Runtime Status Funnel

```mermaid
flowchart TB
    pending["pending"] --> assigned["assigned"]
    assigned --> pulling["pulling"]
    pulling --> created["created"]
    created --> running["running"]
    running --> healthy["healthy"]
    running --> unhealthy["unhealthy"]
    unhealthy --> failed["failed"]
    running --> stopping["stopping"]
    stopping --> stopped["stopped"]
    stopped --> removed["removed"]
```

## Operator Feedback Loop

```mermaid
sequenceDiagram
    participant O as Operator
    participant C as CLI
    participant A as API
    participant S as Scheduler
    participant W as Agent
    participant D as Docker
    O->>C: orch deploy service.yaml
    C->>A: POST /v1/services
    A->>S: place pending tasks
    W->>A: poll assigned tasks
    W->>D: pull/create/start container
    W->>A: report task status
    A-->>C: service + events + logs
```
