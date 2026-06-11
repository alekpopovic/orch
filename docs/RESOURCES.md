# Resources

Resource values are normalized before services are stored:

- CPU is stored in millicores.
- Memory is stored in bytes.

Accepted CPU formats:

- `500m` means 500 millicores.
- `1` means 1000 millicores.
- `2.5` means 2500 millicores.

Accepted memory formats:

- `128Mi` and `512Mi` use binary mebibytes.
- `1Gi` uses binary gibibytes.
- `2G` uses decimal gigabytes.

Explicit resource values must be positive. Missing request or limit values are filled from configurable defaults. The current default is:

- request CPU: `100m`
- request memory: `128Mi`
- limit CPU: `100m`
- limit memory: `128Mi`

Requests must not exceed limits after defaults are applied.

## Scheduler Accounting

The scheduler uses service resource requests, not limits, when deciding whether a task fits on a node. Node capacity is measured from `node.allocatable`.

Allocated resources are calculated from every assigned non-terminal task on the node, including tasks that are assigned, pulling, starting, running, healthy, unhealthy, or stopping. Tasks stop consuming scheduling capacity only after they reach a terminal status such as `stopped`, `removed`, or `failed`.

This avoids overcommitting CPU and memory while containers are still being created, running, or torn down.
