# Plugin system design

Plugins extend runtime, storage, secrets, registry, policy, metrics, and notification boundaries without coupling reconciliation to vendors. This is not a general code-loading framework and does not accept arbitrary untrusted in-process binaries.

Built-ins remain in-process behind narrow interfaces. Third-party plugins run as least-privilege processes over versioned RPC on Unix sockets or mutually authenticated TCP. This isolates crashes and dependencies and permits independent upgrades. Runtime/storage calls use request IDs and idempotency keys.

Handshake data contains protocol version, plugin type, implementation version, capabilities, and supported orch range. Major mismatch fails startup; capabilities allow additive evolution. Configuration names an instance, endpoint/executable, type, non-secret settings, secret references, timeouts, and restart policy. Secrets are scoped and never returned by status APIs.

The supervisor validates, starts/connects, handshakes, health-checks, drains calls, and stops. Enforce resource/process limits, filesystem/network restrictions, signed artifacts, socket ownership, mTLS, and deny-by-default host access. Runtime/storage plugins remain part of the trusted computing base.

Observe latency, errors, restarts, protocol/capability data, health, and redacted logs. Contract suites cover every type; fake plugins cover timeout, malformed output, crash, replay/idempotency, mismatch, backpressure, and redaction. Examples should include an external notification sink and read-only registry provider.
