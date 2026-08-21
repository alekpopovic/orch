# Policy Engine Design

## Goals

The policy engine gives `orch` one deterministic admission boundary for workload safety and organizational rules. It evaluates a normalized service spec together with actor, operation, namespace, and cluster policy before state is persisted or changed. Decisions are structured, auditable, testable without Docker or PostgreSQL, and reusable by create, update, rollout, and scale flows.

The initial controls cover required resource requests/limits, privileged containers, host namespaces, Linux capabilities, approved image registries, mutable tags, healthchecks, host paths, replica ceilings, and public-port ceilings.

## Non-goals

The engine does not replace schema/domain validation, runtime authorization, image scanning, network policy, secret encryption, or agent-side defense in depth. It is not a general workflow engine and the MVP does not execute Rego or make network calls during admission.

## Enforcement Points

| Operation | Required decision |
| --- | --- |
| Service create/update | Validate the complete normalized candidate before persistence. |
| Rollout | Copy the current spec, apply the candidate image, then admit it before creating a deployment. |
| Scale | Copy the current spec, apply candidate replicas, then enforce replica policy. |
| Node drain | Future dynamic policy may restrict disruption windows or protected workloads. |
| Secret usage | Resolve references only inside the service namespace; future rules can allow named cross-namespace grants. |
| Registry credentials | Resolve credentials only inside the service namespace and validate registry policy independently. |
| Agent execution | Agents revalidate security-sensitive task settings before Docker execution as defense in depth. |

Schema validation runs first. Admission then returns all policy violations, each with stable `field`, `rule`, and `message` values. A rejected request is not persisted and writes a redacted `admission.<operation>` failure record to the audit log.

## Example Policies

```yaml
cluster_policy:
  require_resource_requests: true
  require_resource_limits: true
  allow_privileged: false
  allow_host_network: false
  allow_host_pid: false
  allowed_image_registries: [ghcr.io, registry.example.com]
  block_latest_tag: true
  require_healthcheck: true
  allowed_host_path_prefixes: [/var/lib/orch-volumes]
  max_replicas_per_service: 50
  max_public_ports_per_service: 4
```

This requires CPU/memory requests and limits, blocks privileged and host-namespace access, accepts only approved registries, rejects implicit or explicit `latest`, requires a healthcheck, confines host mounts, and bounds replicas/public ports.

## Static and Dynamic Policy

Static policy is loaded with server configuration, has no external dependency, and is the authoritative MVP path. A future dynamic provider may select policy by namespace, actor, labels, or environment. Dynamic decisions need bounded timeouts, cached last-known-good bundles, revision identifiers in audit records, and an explicit fail-open/fail-closed setting; production security policy should default to fail closed.

## OPA/Rego Evolution

OPA integration should implement the same admission evaluator boundary. Input is a versioned document containing actor, operation, namespace, normalized service spec, and non-secret cluster facts. Output is a list of the same structured violations plus policy revision. Secrets and registry passwords are never policy input. Rego bundles must be signed, validated, and activated atomically.

## Audit and Testing

Audit records contain actor, namespace, operation, service name, outcome, and violated rule IDs—never secret values or credentials. Unit tests cover each built-in rule, allowlist boundaries, path-prefix traversal, image parsing, multiple simultaneous violations, and rejection auditing. Control-plane tests prove create/scale/rollout cannot bypass admission; API tests verify stable structured errors. Future OPA adapters need conformance tests against the built-in decision contract.
