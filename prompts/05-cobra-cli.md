# 05. Cobra CLI

```text
Implement the initial orch CLI using Cobra.

Commands:

orch version

orch node ls
orch node inspect <node-id>
orch node drain <node-id>
orch node uncordon <node-id>

orch deploy <file.yaml>
orch service ls
orch service inspect <service-name-or-id>
orch service ps <service-name-or-id>
orch scale <service-name-or-id> --replicas <n>
orch rollout <service-name-or-id> --image <image>
orch rollback <service-name-or-id>
orch delete <service-name-or-id>

orch events
orch logs <service-name-or-id>

Requirements:
- Read server URL from:
  1. --server flag
  2. ORCH_SERVER_URL env var
  3. config file
- Support JSON and table output:
  --output table
  --output json
- Implement YAML parsing for deploy files.
- Validate deploy files before sending to API.
- Add useful human-readable error messages.
- Add examples to README.md.
- Add tests for YAML parsing and CLI command construction.

Example deploy YAML:

name: api
image: ghcr.io/example/api:1.0.0
replicas: 3
ports:
  - container: 8080
    public: 80
env:
  NODE_ENV: production
resources:
  cpu: 500m
  memory: 512Mi
healthcheck:
  type: http
  path: /health
  interval: 10s
  timeout: 2s
restart:
  policy: always
placement:
  labels:
    role: app
```
