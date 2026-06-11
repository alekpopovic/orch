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

Agent authentication is separate from user authentication. Agent endpoints under `/v1/agent/*` continue to use the static bootstrap token configured by `ORCH_BOOTSTRAP_TOKEN`.

Do not reuse user JWT secrets as agent bootstrap tokens.

## Logging

The API access log records request ID, method, path, status, and duration. It does not log `Authorization` headers, JWTs, bootstrap tokens, or request bodies.

## MVP Limitations

- Users are represented by signed JWT claims plus optional `ORCH_USERS` static config; there is no database-backed user management yet.
- Token issuance is external to the server for now.
- Use a long, random `ORCH_JWT_SECRET` and rotate it through your deployment secret manager.
- mTLS should replace static bootstrap-token agent identity before production multi-tenant use.
