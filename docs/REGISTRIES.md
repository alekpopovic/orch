# Image Registries

`orch` can store encrypted image registry credentials and send them only to agents that receive tasks for services referencing those credentials.

## Create A Credential

```sh
curl -X POST http://localhost:8080/v1/registry-credentials \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "ghcr-prod",
    "registry": "ghcr.io",
    "username": "robot",
    "password": "ghp_example_token"
  }'
```

The password/token is encrypted with the server envelope key before storage. API list responses include metadata only:

```sh
curl http://localhost:8080/v1/registry-credentials
```

Delete a credential:

```sh
curl -X DELETE http://localhost:8080/v1/registry-credentials/ghcr-prod
```

## Service YAML

Reference the credential by ID:

```yaml
name: private-api
image: ghcr.io/example/private-api:1.0.0
imagePullSecret: ghcr-prod
replicas: 1
resources:
  cpu: 100m
  memory: 128Mi
```

The control plane rejects services that reference missing credentials. Agents receive pull credentials only in assigned task payloads for the service that needs them, and the Docker runtime passes them to `PullImage`.

## Security Notes

- Do not log registry passwords or tokens.
- Keep `ORCH_SECRET_KEY` stable; it protects registry credentials and secrets.
- Use registry-scoped robot accounts or tokens with least privilege.
