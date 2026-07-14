# 88. Performance profiling

```text
Add performance profiling support.

Context:
Controllers may become slow as services, tasks, nodes, and events grow.

Task:
Add optional pprof and benchmark suite.

Requirements:
- Add config option to enable pprof only on admin/debug listener.
- Never expose pprof publicly by default.
- Add benchmarks for:
  - scheduler scoring
  - reconciliation loop for many services
  - event insertion
  - service list API
  - task list API
- Add docs/PERFORMANCE.md:
  - how to run benchmarks
  - how to capture profiles
  - safety warning for pprof
- Add tests ensuring debug endpoints are disabled by default.

At the end:
- Run go test ./...
```
