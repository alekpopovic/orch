# SBOM and Vulnerability Scanning Design

## Scope and Architecture

Supply-chain state is keyed by immutable image digest and remains compatible with the current registry credential, image-resolution, and admission flow. A digest may have multiple SBOM documents and scan results, but admission selects the newest trusted, non-expired result for the configured scanner/policy.

The system ingests SPDX or CycloneDX SBOM metadata from OCI referrers, CI upload, or a registry scanner. Large documents remain in an object store/registry; PostgreSQL stores digest, format, document hash, location, producer identity, timestamps, and verification status. Ingestion never accepts a tag as artifact identity.

Scanner adapters submit or discover a digest, normalize findings, and record scanner/database versions. Registry-native scanners can push status through a webhook or be polled. Registry credentials are resolved through the existing encrypted credential boundary and are never copied into scan records, events, or jobs.

## Proposed Data Model

- `image_artifacts`: digest, registry/name, first/last seen, provenance reference.
- `image_sboms`: artifact digest, format, document digest/location, producer, verified_at.
- `vulnerability_scans`: artifact digest, scanner, DB version, state, summary counts, scanned/expires timestamps.
- `vulnerability_findings`: scan ID, vulnerability ID, package/version, severity, fix availability.
- `supply_chain_waivers`: namespace, digest or vulnerability ID, reason, approver, expiry.

Scanner payloads are bounded, validated, and idempotent by `(digest, scanner, database_version)`.

## Admission Policy

Example:

```yaml
supply_chain:
  require_sbom: true
  require_recent_scan: 24h
  deny_severities: [critical]
  deny_fixable_high: true
  allowed_scanners: [trivy-prod]
```

Admission runs after tag-to-digest resolution. Missing/stale data, active scans, scanner outage, and policy violation are separate structured results. Production defaults to fail closed for missing or expired evidence. Development may allow deployment while a scan is pending, but this produces an audit event.

## Exceptions and Audit

Waivers are namespace-scoped, least-specific-last (digest plus vulnerability ID is preferred), time-bounded, and require reason, approver, and ticket reference. They cannot waive an unknown digest. Creation, use, expiry, and revocation are audited. Admission events record digest, scan ID, policy revision, maximum severity, and waiver ID without embedding the SBOM or registry credentials.

## Periodic Rescan and Running Images

A scheduler rescans active digests when the vulnerability database changes or a result nears expiry. Work is deduplicated per digest and retried with bounded backoff.

When a running image later violates policy, `orch` emits a high-severity event and marks the service supply-chain status noncompliant. Initial behavior is alert-only: automatic termination can reduce availability and is unsafe without an approved replacement. A future namespace policy may choose quarantine, rollout blocking, or controlled eviction inside a maintenance window.

## API and CLI Proposal

API:

- `POST /v1/supply-chain/sboms`
- `POST /v1/supply-chain/scans`
- `GET /v1/images/{digest}/supply-chain`
- `POST /v1/supply-chain/waivers`
- `DELETE /v1/supply-chain/waivers/{id}`
- `POST /v1/images/{digest}/rescan`

CLI:

```sh
orch image inspect <digest>
orch image sbom upload <digest> --file sbom.spdx.json
orch image scan <digest>
orch image waiver create <digest> --vulnerability CVE-... --expires 24h --reason ...
orch image waiver delete <id>
```

Uploads use streaming size limits and document hashes; CLI output defaults to summaries rather than full finding lists.

## Test Strategy

- Parser/fuzz tests for SPDX, CycloneDX, scanner payloads, severity normalization, and size limits.
- Admission tables for fresh/stale/missing scans, severity thresholds, fixability, and waivers.
- Idempotency and concurrent webhook tests.
- Registry-scanner contract tests with encrypted credential fakes.
- Periodic rescan and later-vulnerable running-image tests with injected clocks.
- Audit redaction, namespace isolation, waiver expiry, and failure-mode tests.
