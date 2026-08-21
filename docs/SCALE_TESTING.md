# Scale Testing

The optional scale scenario creates 1,000 services with fake nodes and fake task transitions, so no Docker containers are started:

```sh
go run ./cmd/orch-loadtest \
  --services 1000 --nodes 50 --replicas 3 \
  --failure-rate 0.01 --duration 30s --seed 42
```

The service, node, replica, failure-rate, duration, and seed parameters are configurable. Output reports convergence duration, total/running/failed tasks, scheduler and reconciliation iterations, their error counts, and average per-service convergence time.

An optional test is excluded from normal CI:

```sh
go test -tags scale ./cmd/orch-loadtest -run TestThousandServiceScale -v
```

Run PostgreSQL alongside a production-shaped test and apply `orch-server migrate up` before collecting results. The current v0.3 development server still keeps live control-plane state in memory, so this fake-agent command measures scheduler/reconciler behavior rather than PostgreSQL throughput. Use a dedicated database and host for persistence baselines; do not compare these numbers as if they were durable-cluster results.

