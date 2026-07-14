# 68. Image signing design

```text
Create a design document for image signing and verification.

Context:
The orchestrator should eventually verify container images before deployment using technologies such as Sigstore/cosign or equivalent.

Task:
Create docs/IMAGE_SIGNING_DESIGN.md.

Cover:
- goals
- non-goals
- threat model
- where verification happens:
  - API admission
  - rollout creation
  - agent before pull/start
- trust roots
- keyless signing option
- key-based signing option
- policy examples:
  - require signed images
  - require specific issuer
  - require specific identity
  - allow unsigned images only in dev namespace
- failure modes
- audit events
- test strategy
- rollout migration path

Do not implement verification yet.
```
