# Testing

Run the deterministic unit and integration-safe suite with:

```sh
go test ./...
```

Scheduler property tests generate many node, task, resource, label, and port combinations from fixed seeds. A failure prints the seed so the same cluster state can be reproduced. They assert ready-node placement, capacity, label and host-port constraints, determinism, and unique assignment.

Go fuzz targets cover service YAML, resource quantities, cron expressions, image references, port specs, label selectors, healthcheck paths, and API request bodies. They do not use Docker or the network. Run one target locally, for example:

```sh
go test ./internal/cli -run '^$' -fuzz FuzzServiceYAML -fuzztime 30s
go test ./pkg/types -run '^$' -fuzz FuzzParsePortSpec -fuzztime 30s
go test ./internal/api -run '^$' -fuzz FuzzCreateServiceRequest -fuzztime 30s
```

Fuzzing is intentionally manual rather than part of normal CI so CI duration remains predictable. Preserve useful minimized inputs as seed corpus entries or regression tests.
