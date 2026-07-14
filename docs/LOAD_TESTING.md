# Load Testing

`orch-loadtest` simulates multi-node orchestration behavior with the in-memory control plane and fake agent transitions. It does not require Docker or PostgreSQL.

## Scenario

The tool:

1. Creates configurable fake nodes.
2. Deploys configurable services.
3. Creates a fixed replica count per service.
4. Simulates assigned tasks transitioning to `running`.
5. Randomly fails running tasks.
6. Randomly marks nodes offline or online through heartbeat updates.
7. Prints a summary of convergence and error counters.

## Usage

```sh
go run ./cmd/orch-loadtest \
  --nodes 10 \
  --services 100 \
  --replicas 3 \
  --failure-rate 0.02 \
  --duration 30s \
  --seed 1234
```

Parameters:

- `--nodes`: number of fake nodes.
- `--services`: number of services to create.
- `--replicas`: replicas per service.
- `--failure-rate`: probability of failing a running task during each simulation tick.
- `--duration`: simulation duration after initial convergence.
- `--seed`: random seed for reproducibility.

## Output

The summary includes:

- `tasks_created`
- `tasks_running`
- `tasks_failed`
- `average_convergence_time`
- `scheduler_errors`
- `reconciler_errors`

This is a fast local smoke/load tool, not a replacement for PostgreSQL or Docker integration tests.
