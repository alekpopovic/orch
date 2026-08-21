# Performance Profiling

Run all benchmarks or a focused benchmark with:

```sh
go test ./internal/... -run '^$' -bench . -benchmem
go test ./internal/scheduler -run '^$' -bench BenchmarkSchedulerScoring -cpuprofile /tmp/orch-scheduler.cpu
go tool pprof /tmp/orch-scheduler.cpu
```

The suite covers scheduler scoring, reconciliation across many services, event insertion, and the service/task list HTTP handlers. Compare results on the same Go version and host, and record workload size with every baseline.

pprof is disabled by default and is never registered on the public listener. To enable its dedicated admin listener:

```sh
ORCH_ENABLE_PPROF=true ORCH_DEBUG_ADDR=127.0.0.1:6060 orch-server
go tool pprof http://127.0.0.1:6060/debug/pprof/profile?seconds=30
```

Profiles may contain request paths, heap contents, and operational metadata. Bind the debug listener to loopback or a protected management network, require host/network access controls, capture only for the needed interval, and disable it afterward.

