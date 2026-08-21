# Storage

The storage MVP supports Docker `local` volumes, claims, and task attachments. A claim referenced by a Service or Job is resolved before task creation; an unbound local claim creates a matching volume. Local topology pins later use to its first node.

`ReadWriteOnce` permits one active writer unless `allow_concurrent_writers` is explicitly enabled. Read-only mounts do not consume the writer lock. Attachments detach when a Task is removed or Job deleted. A volume with an active attachment cannot be deleted.

Use `orch volume create <name> [--node ID]`, `orch volume ls`, and `orch volume inspect <id>`. Distributed storage, replication, snapshots and unsafe force-detach are intentionally outside this MVP.
