# Secrets

`orch` supports an MVP secret store for environment-variable injection without putting plaintext values in service YAML, service metadata responses, or events.

## Configuration

Set a stable server-side encryption key:

```sh
export ORCH_SECRET_KEY='replace-with-a-long-random-value'
```

The MVP uses a local envelope provider backed by AES-GCM. The provider interface is intentionally small so a future Vault/KMS-backed implementation can replace the local master key without changing service specs or agent task payloads.

## API

Create or update a secret:

```sh
curl -X POST http://localhost:8080/v1/secrets \
  -H 'Content-Type: application/json' \
  -d '{"name":"prod/database-url","value":"postgres://user:pass@db/app"}'
```

List and read metadata:

```sh
curl http://localhost:8080/v1/secrets
curl http://localhost:8080/v1/secrets/prod%2Fdatabase-url
```

Delete a secret:

```sh
curl -X DELETE http://localhost:8080/v1/secrets/prod%2Fdatabase-url
```

GET endpoints never return plaintext values. Secret create/delete events contain only metadata-level messages.

## Service YAML

Use `secretRef` inside `env`:

```yaml
name: api
image: ghcr.io/example/api:1.0.0
replicas: 1
env:
  DATABASE_URL:
    secretRef: prod/database-url
  NODE_ENV:
    value: production
resources:
  cpu: 100m
  memory: 128Mi
```

The CLI keeps literal env values in the service spec and stores secret references separately. Agents receive plaintext only in their assigned task payload when a task needs to start a container.

## Operational Notes

- Keep `ORCH_SECRET_KEY` stable across server restarts and restores.
- Rotating the local key requires re-creating or re-encrypting stored secrets.
- Do not put secret values in service names, event messages, labels, route hostnames, or failure reasons.
- External Vault/KMS integration is intentionally deferred.
