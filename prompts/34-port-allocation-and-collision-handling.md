# 34. Port allocation and collision handling

```text
Implement host port allocation and collision handling.

Context:
Services may request public host ports. The orchestrator must avoid scheduling two tasks with the same host port on the same node.

Task:
Add port allocation awareness to scheduler and Docker runtime.

Requirements:
- Service ports may specify:
  - container port
  - public host port
  - protocol tcp/udp
- Scheduler must reject nodes where requested host port is already allocated.
- Dynamic host port allocation should be supported if public port is omitted or set to 0.
- Store assigned host ports on task records.
- Agent must use assigned ports when creating containers.
- If Docker reports port already allocated, agent reports a structured failure reason.
- Reconciler should retry on another node if possible.
- Add tests for:
  - two services requesting same port on same node
  - same port allowed on different nodes
  - dynamic port assignment
  - Docker port allocation failure
- Update docs/NETWORKING.md.

Do not build overlay networking yet.
```
