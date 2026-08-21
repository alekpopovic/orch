# Image Signing and Verification Design

## Goals and Non-goals

The goal is to prevent untrusted container artifacts from reaching a node by verifying an immutable image digest against namespace policy. Verification must be repeatable, auditable, independent of mutable tags, and available at more than one enforcement point. Sigstore/cosign-compatible signatures are the initial target, but the verifier remains an interface so another OCI-signature implementation can be substituted.

This design does not implement signing, run a certificate authority, mirror registries, scan images for vulnerabilities, or make a signature prove that the image is safe. SBOM and vulnerability decisions are covered by `SUPPLY_CHAIN_SECURITY_DESIGN.md`.

## Threat Model

The controls address registry tag replacement, compromised publisher credentials, an unauthorized publisher, artifact substitution between admission and pull, stale verification results, and policy bypass through a direct agent request. They do not protect a node after host/root compromise, a malicious configured trust root, or a compromised build identity that legitimately satisfies policy.

## Enforcement Points

Verification uses the resolved digest, never a tag:

1. API admission resolves the requested reference and rejects immediately when policy or a cached verification result cannot be satisfied.
2. Rollout creation re-verifies the exact service-version digest, preventing a stale admission decision from silently changing artifacts.
3. The agent verifies the assigned digest before pull/start. This is the final fail-closed boundary and protects against control-plane bypass or stale assignments.

The control plane records the verification subject, policy revision, result, verifier version, and expiry. Agents receive a short-lived signed verification assertion rather than registry or signing credentials.

## Trust Roots and Signing Modes

Key-based signing stores public keys or KMS-backed public-key references in a versioned trust-root set. Private keys never enter `orch`.

Keyless signing validates the Fulcio certificate chain, Rekor inclusion proof, certificate validity at signing time, OIDC issuer, and workload identity. Offline verification uses a periodically refreshed, authenticated transparency-log checkpoint. Network failure behavior is policy-controlled but production namespaces default to fail closed.

Trust-root updates are admin-only, append audited revisions, and overlap old/new roots during rotation. Removing a root triggers re-evaluation of affected running digests.

## Policy Examples

```yaml
image_signing:
  require_signed_images: true
  allowed_issuers:
    - https://token.actions.githubusercontent.com
  allowed_identities:
    - https://github.com/acme/api/.github/workflows/release.yml@refs/heads/main
```

```yaml
namespaces:
  dev:
    image_signing:
      require_signed_images: false
```

An unsigned exception is namespace-scoped, time-bounded, requires a reason/approver, and is never inherited by another namespace.

## Failure Modes and Audit

Invalid signature, untrusted issuer/identity, missing Rekor proof, digest mismatch, expired assertion, unavailable verifier, and trust-root refresh failure are distinct stable failure reasons. Denials emit `image.verification.denied`; successful rollout verification emits `image.verification.succeeded`; exceptions emit `image.verification.waiver_used`. Events and audit records contain the digest and policy revision but no credentials or raw identity tokens.

Retryable verifier/registry outages return an unavailable result. Production policy fails closed; an explicitly configured development policy may use a recent unexpired cached success.

## Test Strategy

- Table tests for issuer, identity, key rotation, expiry, Rekor proof, and digest mismatch.
- Fake verifier tests at API, rollout, and agent boundaries.
- Replay tests proving a verification assertion cannot authorize another digest/namespace.
- Integration tests with locally generated cosign keys and an OCI fixture registry.
- Failure-injection tests for registry, transparency log, clock skew, and trust-root refresh.

## Rollout Path

1. Observe-only verification records results without denial.
2. Require digest pinning while signature policy remains observe-only.
3. Enforce signing for selected non-production namespaces.
4. Enforce production with a temporary, audited waiver mechanism.
5. Enable mandatory agent-side assertion verification and remove observe-only compatibility paths.
