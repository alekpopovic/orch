# Security

`orch` separates user authentication from agent authentication. The current model is appropriate for local and controlled environments; production hardening requires the roadmap items listed below.

The latest repository security review is documented in `docs/SECURITY_REVIEW.md`.

## User API Authentication

User-facing REST API endpoints support JWT authentication when `ORCH_JWT_SECRET` is configured on `orch-server`.

If `ORCH_JWT_SECRET` is empty, user auth is disabled. This is the default in local Compose.

JWTs use HMAC SHA-256 (`HS256`) and should include:

- `sub`: user identifier.
- `role`: `admin`, `operator`, or `viewer`.
- `exp`: expiration timestamp.

Tokens may use `namespace_roles` instead of a cluster-wide `role`. Each map entry grants `viewer`, `operator`, or `admin` only in that namespace. Namespace administration itself still requires a cluster-wide admin.

Example claims:

```json
{
  "sub": "alice",
  "role": "operator",
  "exp": 1791715200
}
```

Optional static user config:

```sh
ORCH_USERS=alice:admin,bob:operator,carol:viewer
```

When `ORCH_USERS` is set, the JWT `sub` must match a configured user and the configured role is authoritative.

## Roles

- `viewer`: read/list nodes, services, tasks, events, and logs.
- `operator`: viewer access plus deploy, scale, rollout, rollback, and delete services.
- `admin`: operator access plus node drain/uncordon and future cluster/user administration.

Roles are hierarchical.

## Agent Authentication

Agent auth is separate from user JWT auth.

1. Agent registers with `Authorization: Bearer <ORCH_AGENT_REGISTRATION_TOKEN>`.
2. Server issues a short-lived agent credential.
3. Server stores only a hash of the issued credential.
4. Agent uses the credential for heartbeat, task polling, and task status updates.
5. Heartbeat responses rotate the credential.
6. Revoked nodes cannot authenticate with their current credential.

Never log or store raw tokens.

## Docker Socket Risk

The agent needs Docker Engine access. Mounting `/var/run/docker.sock` gives the agent broad control over the node. Treat agent compromise as node compromise.

Recommended controls:

- Run agents only on trusted nodes.
- Restrict access to the agent HTTP port.
- Keep the agent image minimal and pinned.
- Do not expose the Docker socket to service containers.
- Monitor Docker operation error metrics and audit task events.

## Healthcheck SSRF Controls

Agents run HTTP and TCP healthchecks from the node. To avoid service specs causing arbitrary host-local probes, healthchecks are only performed against published TCP ports assigned to the task. A configured container port is resolved to the assigned published host port before probing. Unassigned, unpublished, or UDP-only ports are skipped.

HTTP healthchecks accept only a scheme and path, not arbitrary URLs. The agent constructs the target from task networking data, blocks redirects away from the original endpoint, and limits response body reads.

## Container Security Policy

Services may set `security_context` / `securityContext` fields for runtime hardening, including an explicit non-root `user`, read-only root filesystem, Linux capability drops/additions, host network, host PID, and host path mounts.

The default cluster policy is deny-by-default for privilege-escalating options:

- privileged containers are rejected;
- host network and host PID namespace are rejected;
- added Linux capabilities are rejected unless explicitly allowlisted;
- arbitrary host path mounts are rejected unless their host path starts with an allowlisted prefix;
- containers drop `NET_RAW` by default at the Docker runtime when no custom capability drop list is provided.

Configure allowlists only for trusted clusters:

```yaml
cluster_policy:
  allow_privileged: false
  allow_host_network: false
  allow_host_pid: false
  allowed_capabilities:
    - NET_BIND_SERVICE
  allowed_host_path_prefixes:
    - /var/lib/orch-volumes
  allowed_image_registries:
    - ghcr.io
  block_latest_tag: true
  require_healthcheck: true
  require_resource_requests: true
  require_resource_limits: true
  max_replicas_per_service: 50
  max_public_ports_per_service: 4
```

The admission controller applies these rules on service creation, rollout, and scale. Rejections return structured rule violations and emit a redacted audit record. See `docs/POLICY_ENGINE_DESIGN.md`.

Equivalent environment variables are `ORCH_POLICY_ALLOW_PRIVILEGED`, `ORCH_POLICY_ALLOW_HOST_NETWORK`, `ORCH_POLICY_ALLOW_HOST_PID`, `ORCH_POLICY_ALLOWED_CAPABILITIES`, and `ORCH_POLICY_ALLOWED_HOST_PATH_PREFIXES`.

## Secret Handling

Service specs support secret references for environment variables. Secret values are encrypted at rest with the local envelope provider and are returned to agents only in assigned task payloads when needed to start containers. Do not put secret values in service names, labels, route hostnames, logs, or events.

Secrets and registry credentials are namespace-scoped. Workloads cannot resolve a reference from another namespace.

Roadmap:

- Integrate a secret manager.
- Encrypt sensitive persisted data.
- Add server-side redaction for any future secret-bearing fields.

## Audit Logging

Mutating user and agent actions are written to a dedicated audit log separate from orchestration events. Audit records include actor type and ID, action, target, request ID, source IP, timestamp, outcome, and redacted metadata.

Captured actions include service create/delete, scale, rollout, rollback, node drain/uncordon, secret and registry credential create/delete, agent registration, and agent token rotation. Secret plaintext, registry passwords, bearer tokens, and credential material must never be stored in audit metadata.

Admin users can search audit records through `GET /v1/audit` or `orch audit`.

## Logging

The API access log records request ID, method, path, status, duration, and route. It must not log:

- `Authorization` headers;
- JWTs;
- registration tokens;
- agent credentials;
- request bodies containing env or secret references.

## Future mTLS Design

The agent credential code is behind issuer/validator interfaces so mTLS can be added later.

Target design:

1. Provision each node with a client certificate or SPIFFE identity.
2. Bind certificate identity to node ID.
3. Validate certificate chain and revocation in agent middleware.
4. Optionally require both mTLS and short-lived token credentials during migration.
5. Remove static registration tokens after node identity is automated.

## Production Requirements Before Multi-Tenant Use

- Wire `orch-server` to PostgreSQL durable state.
- Enable JWT auth with a strong rotated secret.
- Replace static registration bootstrap with mTLS or one-time enrollment.
- Add TLS termination for server and agent endpoints.
- Add audit export and retention.
- Add secret manager integration.
- Add network policy/firewall rules around server, agent, metrics, and Docker access.
