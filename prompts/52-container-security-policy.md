# 52. Container security policy

```text
Implement default container security policy.

Context:
By default, orchestrated containers should be safer than raw Docker defaults.

Task:
Add security options to service spec and runtime.

Defaults:
- run as non-root when image supports it, or allow explicit user setting
- no privileged containers by default
- read-only root filesystem optional
- drop Linux capabilities by default where safe
- allow explicit capability add only through policy
- no host network by default
- no host PID namespace by default
- no arbitrary hostPath mounts by default

Requirements:
- Add ServiceSpec securityContext.
- Validate unsafe options.
- Add cluster policy config:
  - allowPrivileged
  - allowHostNetwork
  - allowHostPID
  - allowedHostPathPrefixes
  - allowedCapabilities
- API should reject disallowed specs.
- Add tests for policy enforcement.
- Update docs/SECURITY.md and docs/SERVICE_SPEC.md.
```
