# 38. Image registry authentication

```text
Implement image registry authentication.

Context:
Services may use private images from registries such as ghcr.io, Docker Hub, GitLab Registry, or a private registry.

Task:
Add registry credentials support.

Requirements:
- Add RegistryCredential model:
  - registry host
  - username
  - encrypted password/token
  - created/updated timestamps
- Add API endpoints:
  - POST /v1/registry-credentials
  - GET /v1/registry-credentials
  - DELETE /v1/registry-credentials/{id}
- Service YAML may reference a credential:
  imagePullSecret: ghcr-prod
- Agent should receive pull credentials only for assigned tasks.
- Docker runtime should use credentials for PullImage.
- Never log credentials.
- Add tests:
  - credential create/delete
  - service references missing credential
  - agent receives correct auth
  - Docker pull gets auth config
- Update docs/REGISTRIES.md.
```
