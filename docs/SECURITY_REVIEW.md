# Security Review

Date: 2026-07-14

Scope: Docker socket exposure, agent authentication, user authentication, RBAC, secrets and registry credentials, API input validation, audit logging, log redaction, path traversal, command injection, SSRF through healthchecks, unsafe container options, privilege escalation, and default container security profile.

## Executive Summary

One high-risk issue was found and fixed in this review: service-defined healthchecks could cause agents to probe arbitrary host-local TCP ports on `127.0.0.1`. The fix restricts healthchecks to ports assigned to the task.

No direct command-injection or path-traversal issue was found in the reviewed code. The remaining material risks are mostly architectural production-hardening items: Docker socket exposure, bootstrap-token based agent log access, and local-development auth defaults.

## Prioritized Findings

### High: Healthcheck Host-Local SSRF

Before this review, an operator-controlled service spec could define a healthcheck port that was unrelated to the task's assigned ports. The agent built probes against `127.0.0.1:<port>`, so a compromised operator account could make every agent probe host-local services such as admin HTTP endpoints, local metadata proxies, or an accidentally exposed Docker TCP API.

Exploit scenario:

1. Attacker obtains an operator JWT or compromises an operator workflow.
2. Attacker deploys a service with a healthcheck targeting a sensitive host-local port, for example `2375` or an internal admin service.
3. Agents perform repeated HTTP/TCP probes to `127.0.0.1:<target>`.
4. Even with GET-only HTTP probes, the attacker can discover port availability and potentially trigger unsafe local endpoints.

Minimal fix:

- The agent now accepts healthcheck probes only when the requested port matches the task's assigned published TCP container or host port.
- Container ports are resolved to the assigned published host port before probing.
- Unassigned, unpublished, or UDP-only healthcheck ports are skipped instead of probed.

Tests:

- `internal/agent/agent_test.go` includes `TestHealthProbeRequiresAssignedPort`.
- Existing healthcheck threshold tests now include assigned ports and continue to pass.

Documentation updates:

- `docs/HEALTHCHECKS.md` documents assigned-port enforcement.

### Medium: Docker Socket Exposure

The agent requires Docker Engine access. Access to `/var/run/docker.sock` is equivalent to broad host control in most deployments.

Minimal fix:

- No code-only fix is appropriate without replacing the runtime isolation model.
- Production examples and docs warn about this explicitly and restrict the agent to trusted nodes.

Recommended tests:

- Add integration tests for a future restricted runtime adapter or authorization proxy.

Documentation updates:

- `docs/SECURITY.md`, `docs/PRODUCTION_DEPLOYMENT.md`, and `docs/AGENT.md` describe the risk.

### Medium: Bootstrap Token Used For Agent Log Endpoint

The server proxies logs from agents using the registration/bootstrap token. If this token leaks, it can be reused against agent log endpoints until rotated.

Minimal fix:

- Replace bootstrap-token log access with mTLS or a short-lived server-to-agent credential scoped to log streaming.
- Restrict agent ports to the control-plane network until that change lands.

Recommended tests:

- Add tests for credential expiry, wrong-node rejection, and token rotation for server-to-agent log streaming.

Documentation updates:

- `docs/LOGS.md` already calls out the limitation.

### Medium: User Auth Is Optional In Development Defaults

If `ORCH_JWT_SECRET` is empty, user-facing API auth is disabled. This is useful locally but unsafe for exposed environments.

Minimal fix:

- Keep the developer default, but require production manifests and runbooks to set `ORCH_JWT_SECRET`.
- Consider adding an explicit `ORCH_ENV=production` guard that refuses to start without JWT auth.

Recommended tests:

- Add config validation tests for a future production mode.

Documentation updates:

- `docs/SECURITY.md` and `docs/PRODUCTION_DEPLOYMENT.md` require JWT auth in production.

### Medium: Secret And Registry Credential Blast Radius

The server decrypts secrets and registry credentials for agents when a task needs them. A compromised agent can read secrets for tasks assigned to that node.

Minimal fix:

- Keep least-privilege secret references per service.
- Use registry-scoped robot credentials.
- Prefer a secret manager and node-bound delivery in future work.

Recommended tests:

- Continue regression tests proving API responses never return plaintext secrets or registry passwords.

Documentation updates:

- `docs/SECRETS.md`, `docs/REGISTRIES.md`, and `docs/SECURITY.md` describe handling rules.

### Low: Unsafe Container Options

The current service spec does not expose privileged mode, host networking, host PID/IPC, arbitrary mounts, additional capabilities, or device mappings. Docker container creation sets no privileged flags and uses explicit resource/port fields.

Minimal fix:

- Keep these options out of the public service spec unless a future security policy gate is added.

Recommended tests:

- Add regression tests if container security policy becomes configurable.

### Low: Command Injection And Path Traversal

No direct shell execution was found for orchestration operations. File path usage in the CLI is limited to user-selected local config/deploy files. The Docker runtime uses typed API calls rather than shell commands.

Minimal fix:

- Continue avoiding shell execution for orchestration.
- Keep path-taking features scoped to explicitly provided files.

### Low: Audit Logging

Audit logs are stored separately from operational events and redact sensitive metadata. The current implementation is best-effort for write operations.

Minimal fix:

- For high-compliance deployments, make audit append failures fail closed for selected admin operations after durable store wiring is complete.

## Fixes Implemented In This Review

- Restricted agent healthchecks to task-assigned ports.
- Added regression coverage for rejected arbitrary host-local healthcheck ports.
- Updated healthcheck documentation to explain the SSRF control.

## Follow-Up Backlog

1. Replace bootstrap-token log streaming with mTLS or scoped short-lived credentials.
2. Add a production-mode config guard requiring JWT auth, non-default secret keys, and non-default bootstrap tokens.
3. Add a container security policy before exposing advanced Docker options.
4. Add current-state metrics for node and task counts so dashboards and alerts no longer rely on scrape status/counters as proxies.
