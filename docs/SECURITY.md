# Security

`orch` separates user authentication from agent authentication. The current model is appropriate for local and controlled environments; production hardening requires the roadmap items listed below.

## User API Authentication

User-facing REST API endpoints support JWT authentication when `ORCH_JWT_SECRET` is configured on `orch-server`.

If `ORCH_JWT_SECRET` is empty, user auth is disabled. This is the default in local Compose.

JWTs use HMAC SHA-256 (`HS256`) and should include:

- `sub`: user identifier.
- `role`: `admin`, `operator`, or `viewer`.
- `exp`: expiration timestamp.

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

## Secret Handling

Service specs support secret references, not plaintext secret storage. Do not put secret values in service env vars, deploy YAML, logs, or events.

Roadmap:

- Integrate a secret manager.
- Encrypt sensitive persisted data.
- Add server-side redaction for any future secret-bearing fields.

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
