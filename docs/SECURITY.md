# Security

## User API Authentication

User-facing REST API endpoints support JWT authentication when `ORCH_JWT_SECRET` is configured on `orch-server`.

Tokens use HMAC SHA-256 (`HS256`) and must include:

- `sub`: user identifier.
- `role`: one of `admin`, `operator`, or `viewer`.
- `exp`: recommended expiration timestamp.

Example claims:

```json
{
  "sub": "alice",
  "role": "operator",
  "exp": 1791715200
}
```

The server validates the signature, role, and expiration before applying RBAC.

For MVP static users can be configured with `ORCH_USERS`:

```sh
ORCH_USERS=alice:admin,bob:operator,carol:viewer
```

When `ORCH_USERS` is set, the JWT `sub` must match a configured user and the configured role is authoritative. This prevents a token from escalating itself by changing the `role` claim.

## Roles

- `viewer`: list/read nodes, services, tasks, events, and logs.
- `operator`: viewer access plus deploy, scale, rollout, rollback, and delete services.
- `admin`: operator access plus node drain/uncordon and future cluster/user administration.

Roles are hierarchical: `admin` includes `operator` and `viewer`; `operator` includes `viewer`.

## Agent Authentication

Agent authentication is separate from user authentication.

The registration token configured by `ORCH_AGENT_REGISTRATION_TOKEN` is only used for registration. An agent presents it to `POST /v1/agent/register`. After successful registration, the server issues a short-lived agent credential. The server stores only a hash of that credential on the node record, never the raw token.

Agents use the issued credential for:

- `POST /v1/agent/heartbeat`
- `GET /v1/agent/tasks`
- `POST /v1/agent/tasks/{task_id}/status`

Heartbeat responses rotate the credential. The new credential replaces the previous hash, so older credentials stop working. Nodes also have a server-side revocation flag; revoked nodes cannot authenticate with their current credential.

Do not reuse user JWT secrets as agent registration tokens or agent credentials.

## Future mTLS Design

The agent auth implementation is intentionally behind credential issuer/validator interfaces. A future mTLS authenticator can be added beside or instead of token auth:

1. Issue each node a client certificate at registration or through external provisioning.
2. Bind the certificate subject or SPIFFE ID to the node ID.
3. Validate certificate chains and revocation status in middleware.
4. Optionally require both mTLS identity and short-lived token credentials during migration.
5. Remove static registration tokens once node identity is fully automated.

The API should continue to treat user JWT auth and agent identity as separate middleware chains.

## Logging

The API access log records request ID, method, path, status, and duration. It does not log `Authorization` headers, JWTs, registration tokens, agent credentials, or request bodies.

## MVP Limitations

- Users are represented by signed JWT claims plus optional `ORCH_USERS` static config; there is no database-backed user management yet.
- Token issuance is external to the server for now.
- Use a long, random `ORCH_JWT_SECRET` and rotate it through your deployment secret manager.
- mTLS should replace or harden registration-token-based agent bootstrap before production multi-tenant use.
