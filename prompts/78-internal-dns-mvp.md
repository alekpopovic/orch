# 78. Internal DNS MVP

```text
Implement a simple internal DNS service MVP.

Context:
Service discovery endpoints exist. Containers should be able to resolve service names to healthy task endpoints.

Task:
Add an optional internal DNS component.

Requirements:
- Add orch-dns binary or internal DNS server inside orch-server if simpler.
- Resolve:
  - <service>.<namespace>.svc.orch
  - <service>.svc.orch
- Return records for healthy running endpoints.
- Support TTL config.
- Add metrics:
  - dns_queries_total
  - dns_errors_total
- Add tests for DNS resolution.
- Add docker-compose example showing container using orchestrator DNS.
- Update docs/SERVICE_DISCOVERY.md and docs/NETWORKING.md.

Keep MVP simple.
Do not implement full Kubernetes DNS compatibility.
```
