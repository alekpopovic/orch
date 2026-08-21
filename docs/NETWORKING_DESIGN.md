# Advanced networking design

The current host-port model is simple and debuggable, with endpoint discovery and optional Traefik configuration. It consumes scarce ports, reveals node topology, requires clients to follow rescheduling, and lacks workload identity and network policy.

Internal DNS is the first migration step: `<service>.<namespace>.svc.orch` and `<service>.svc.orch` return healthy node addresses with a short TTL. It improves naming but provides no virtual IP; clients still use published ports and retry changes.

An overlay could supply routable task IPs through VXLAN or WireGuard and controlled IPAM. CNI integration would invoke established bridge, overlay, and policy plugins through typed, checkpointed results. Calls must be isolated, version-pinned, time-bounded, idempotent, and least-privilege. A native overlay has tighter integration but a larger correctness and security burden.

NetworkPolicy should select namespace/workload labels and define default-deny ingress/egress with explicit peers and ports. DNS/control-plane traffic needs protected system rules. Ingress remains a gateway concern; egress rules require stable identity, IPv4/IPv6 parity, and auditable rejection.

Threats include identity spoofing, malicious plugins, route injection, tenant crossing, DNS poisoning, key leakage, and exhaustion. Encrypt node traffic, rotate keys, validate plugin output, and fail closed. Migrate behind a feature gate: DNS, task addressing, overlay, ingress, then policy. Test namespaces, IPAM properties/fuzzing, partitions, MTU, stale DNS, plugin timeout, conformance, and tenant isolation before retiring host ports.
