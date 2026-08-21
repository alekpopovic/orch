# Notifications

Notification sinks deliver important events to webhook, Slack-compatible webhook, or generic HTTP endpoints. Manage them through `/v1/notification-sinks`; the test subresource performs a real delivery.

Delivery uses JSON, bounded exponential retry, a request timeout, and optional HMAC-SHA256 in `X-Orch-Signature`. Secret-like fields and inline credential markers are redacted. Signing secrets are write-only and must be envelope-encrypted by durable stores.

Notifiable categories include rollout failure, node offline, task failure threshold, admission rejection, quota exhaustion, service/secret deletion, and inability to place a Task. Receivers must deduplicate by event ID because retry delivery is at-least-once.
