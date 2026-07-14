# Chaos Testing

Chaos-style tests exercise reliability behavior without Docker, PostgreSQL, real clocks, or arbitrary sleeps.

## Covered Scenarios

`internal/e2e/chaos_test.go` covers:

1. Agent restarts during task start.
2. Server restarts during rollout.
3. Docker runtime fails to start container.
4. Image pull fails repeatedly.
5. Node goes offline during scale-up.
6. Node returns with stale containers.
7. Database write fails during scheduler assignment.
8. User deletes service during rollout.

## Test Style

- Uses `controlplane.NewMemoryService` and fake scheduler store boundaries.
- Avoids real Docker and PostgreSQL.
- Avoids sleeps; state transitions are driven directly.
- Asserts final safe state and relevant events.
- Keeps failure reasons explicit so regressions are diagnosable.

Run:

```sh
go test ./internal/e2e -run TestChaosScenarios
```

These tests complement lower-level scheduler, reconciler, rollout, and agent unit tests.
