# 79. Plugin system design

```text
Create a design document for orchestrator plugins.

Context:
Future integrations may include storage drivers, registry providers, secret backends, metrics providers, policy engines, and notification sinks.

Task:
Create docs/PLUGIN_SYSTEM_DESIGN.md.

Cover:
- goals
- non-goals
- plugin types:
  - runtime
  - storage
  - secrets
  - registry
  - policy
  - metrics
  - notifications
- in-process vs external plugin model
- version compatibility
- security model
- configuration
- lifecycle
- observability
- test strategy
- examples

Do not implement plugin system yet.
```
