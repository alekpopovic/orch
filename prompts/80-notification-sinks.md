# 80. Notification sinks

```text
Implement notification sinks for important events.

Context:
Operators need alerts outside the orchestrator UI/CLI.

Task:
Add notification sink support.

Supported sinks for MVP:
- webhook
- Slack-compatible webhook if simple
- generic HTTP endpoint

Events to notify:
- rollout failed
- node offline
- task failure threshold exceeded
- admission rejected
- quota exceeded
- service deleted
- secret deleted
- scheduler unable to place task

Requirements:
- Add NotificationSink model.
- Add API:
  - POST /v1/notification-sinks
  - GET /v1/notification-sinks
  - DELETE /v1/notification-sinks/{id}
  - POST /v1/notification-sinks/{id}/test
- Add retry with backoff.
- Add redaction for secrets.
- Add signing secret for webhooks.
- Add tests for delivery success/failure/retry.
- Update docs/NOTIFICATIONS.md.

At the end:
- Run go test ./...
```
