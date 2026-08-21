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

## Image Digests and Pinning

Image references are parsed into the requested reference, registry, repository name, tag, and resolved digest. References that already contain `@sha256:...` are immediately immutable; a configured resolver can resolve a tag during create or rollout. The metadata is stored in the service version and copied to every task.

Image policy supports:

```yaml
cluster_policy:
  require_digest: true
  allow_mutable_tags: false
  deny_latest_tag: true
```

When `require_digest` is enabled, the task image becomes `registry/name@digest`; both Docker pull and container create receive that exact reference. If resolution cannot produce a digest, admission rejects the service or rollout. `block_latest_tag` remains accepted as the compatibility name for `deny_latest_tag`.

The current default parser resolves metadata and existing digest references. Registry-specific remote resolvers plug into the control-plane resolver interface; this MVP does not implement a registry proxy.

## Security Notes

- Do not log registry passwords or tokens.
- Keep `ORCH_SECRET_KEY` stable; it protects registry credentials and secrets.
- Use registry-scoped robot accounts or tokens with least privilege.
- Prefer digest pinning for production namespaces and protect tag mutation in the registry.
