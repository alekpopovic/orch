# 12. Logs support

```text
Implement basic logs support.

Goal:
Allow CLI command:
orch logs <service-name-or-id>
orch logs <service-name-or-id> --follow
orch logs <service-name-or-id> --task <task-id>
orch logs <service-name-or-id> --tail 100

Implementation:
- Agent streams logs from Docker for managed containers.
- Server exposes logs endpoint:
  - GET /v1/logs?service_id=&task_id=&follow=&tail=
- For MVP, proxy logs from agent through server.
- Use chunked HTTP or server-sent events for follow mode.
- Add timeout and cancellation.
- Avoid loading unbounded logs into memory.

Requirements:
- Add tests for log stream cancellation.
- Add error handling when task/node is offline.
- Add docs/LOGS.md explaining MVP limitation.
```
