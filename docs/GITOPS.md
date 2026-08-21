# GitOps Sources

Each applied Service records its GitOps source, commit SHA, repository-relative manifest path, desired spec, drift policy, and last check. `orch gitops status` reports `in_sync`, `drifted`, `unknown`, or `sync_error`; `orch gitops diff <service>` returns live and desired specs.

Manual scale or rollout changes are drift. The default `warn` policy preserves the live change and emits `gitops.drift.detected`. `auto_revert` restores the desired spec and emits `gitops.drift.reverted`. With prune enabled, removing a managed manifest deletes the Service on the next successful sync.

The GitOps controller periodically checks out a Git branch, validates deployment YAML with the same parser and admission path as `orch deploy`, and applies services in the source namespace. Each source records its last revision, managed service names, last sync time, and last error.

## Configure a Source

```sh
orch --namespace payments gitops add \
  --repo https://github.com/acme/orch-config.git \
  --branch main \
  --path services/payments \
  --interval 1m \
  --prune

orch -n payments gitops ls
orch -n payments gitops sync <source-id>
orch -n payments gitops delete <source-id>
```

Equivalent API routes are `POST/GET /v1/gitops/sources`, `POST /v1/gitops/sources/{id}/sync`, and `DELETE /v1/gitops/sources/{id}`.

`path` may identify one `.yaml`/`.yml` manifest or a directory, which is traversed in deterministic lexical order. An update replaces the complete service spec and creates a new service version. Admission, namespace secret resolution, resource quota, and image policy checks cannot be bypassed by GitOps.

With `prune` enabled, a service previously managed by that source is deleted when its manifest disappears. The controller never prunes an unrelated manually created service or a service managed by another source.

## Security and Operations

The MVP uses the installed `git` executable through an isolated fetch adapter and a temporary checkout removed after every sync. Repository URLs containing user info or credential-like query parameters are rejected; credentials are not represented in `GitOpsSource` and cannot be stored in plaintext. Use an external Git credential helper or a future encrypted credential provider for private repositories.

Use immutable commit history protections on the configured branch. A sync records the checked-out commit and emits `gitops.sync.succeeded` or `gitops.sync.failed` plus a system audit record. Repository URLs are redacted by the common audit layer.

The loop respects context cancellation. Failed sources retain their error and retry at their configured interval; one failed source does not stop others.
