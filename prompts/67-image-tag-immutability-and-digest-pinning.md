# 67. Image tag immutability and digest pinning

```text
Implement image digest tracking and optional digest pinning.

Context:
Using mutable image tags can cause inconsistent rollouts. The orchestrator should record the resolved image digest for each task/version.

Task:
Add image digest resolution and storage.

Requirements:
- During rollout or task creation, resolve image tag to digest when possible.
- Store:
  - requested image
  - resolved image digest
  - registry
  - image name
  - tag
- Service version should reference immutable image digest if policy requires it.
- Docker runtime should pull by digest when digest pinning is enabled.
- Add admission policy:
  - requireDigest
  - allowMutableTags
  - denyLatestTag
- Add tests:
  - digest parsing
  - tag parsing
  - rollout stores digest
  - policy rejects mutable tag
  - Docker runtime receives digest-pinned image
- Update docs/REGISTRIES.md and docs/SECURITY.md.

Do not build a full registry proxy.
```
