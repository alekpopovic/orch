# Namespaces and Workload Isolation

Namespaces provide logical workload isolation. `default` always exists and preserves compatibility for clients and data created before namespace support.

Services, tasks, deployments, events, secrets, registry credentials, and audit logs carry a namespace. Service names, secret names, and registry credential IDs are unique within a namespace, so two teams may both deploy a service named `api`. Nodes remain cluster-scoped.

## API and CLI

The API selects a namespace from `X-Orch-Namespace`; if absent it uses `default`. Cross-namespace object IDs are returned as `404` to avoid leaking existence.

```sh
orch namespace create payments
orch namespace ls
orch --namespace payments deploy deployments/examples/http-api.yaml
orch -n payments service ls
orch namespace delete payments
```

The CLI resolves its namespace from `--namespace`, then `ORCH_NAMESPACE`, then `namespace` in its config file, and finally `default`.

Namespace deletion is allowed only when the namespace contains no workload resources or namespace-scoped history. The `default` namespace cannot be deleted.

## RBAC

Legacy JWT `role` remains a cluster-wide role. Namespace-scoped tokens may instead use `namespace_roles`:

```json
{
  "sub": "alice",
  "namespace_roles": {
    "payments": "operator",
    "observability": "viewer"
  }
}
```

Namespace roles are hierarchical (`viewer`, `operator`, `admin`) only inside the named namespace. Namespace creation/list/deletion requires a cluster-wide admin role.

## Secret Boundaries

Secret and registry credential references resolve only in the workload namespace. A service cannot reference a same-named object from another namespace. Explicit cross-namespace grants are intentionally not implemented; adding them requires a separately authorized grant model and audit trail.

## Persistence and Migration

Migration `000011_namespaces` creates the namespace table, assigns existing rows to `default`, replaces global name uniqueness with namespace-local keys, and adds namespace indexes. The down migration removes non-default data before restoring global uniqueness and is therefore intended only for controlled rollback.

Namespaces are a logical isolation boundary, not yet a complete hostile multi-tenant security boundary. Docker nodes, host networking, scheduler capacity, and the current in-memory server process remain shared. Production multi-tenancy still requires PostgreSQL server wiring, network isolation, per-namespace quotas, and stronger agent identity.
