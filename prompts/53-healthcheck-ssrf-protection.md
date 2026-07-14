# 53. Healthcheck SSRF protection

```text
Harden healthchecks against SSRF and unsafe targets.

Context:
Users can define HTTP healthchecks. The agent must only check the assigned container endpoint, not arbitrary URLs.

Task:
Update healthcheck implementation and validation.

Requirements:
- Healthcheck spec should allow path, port, scheme, interval, timeout.
- It must not accept arbitrary full URLs.
- Agent constructs healthcheck target from task/container/node networking data.
- Reject paths with invalid format.
- Prevent redirects to private metadata endpoints or external destinations.
- Add max response body read limit.
- Add tests for:
  - normal /health path
  - full URL rejected
  - redirect handling
  - huge response body
  - timeout
- Update docs/HEALTHCHECKS.md.
```
