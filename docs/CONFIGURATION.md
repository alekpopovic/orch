# Configuration

`orch` loads configuration consistently across `orch-server`, `orch-agent`, and the CLI.

Source precedence is:

1. command flags
2. environment variables
3. YAML config file
4. defaults

## Server

Run with defaults and environment variables:

```sh
orch-server
```

Run with a YAML config file:

```sh
orch-server --config /etc/orch/server.yaml
```

Print the effective server config with secrets redacted:

```sh
orch-server config print --config /etc/orch/server.yaml
```

Example `server.yaml`:

```yaml
addr: :8080
database_url: postgres://orch:orch@postgres:5432/orch?sslmode=disable
log_level: info
bootstrap_token: change-me
jwt_secret: change-me
users: admin:admin
secret_key: change-me
graceful_shutdown_ttl: 10s
heartbeat_timeout: 30s
node_monitor_interval: 5s
cluster_policy:
  require_resource_requests: true
  require_resource_limits: true
  allow_privileged: false
  allow_host_network: false
  allow_host_pid: false
  allowed_image_registries: [registry.example.com]
  block_latest_tag: true
  require_healthcheck: true
  allowed_host_path_prefixes: [/srv/orch]
  allowed_capabilities: [NET_BIND_SERVICE]
  max_replicas_per_service: 20
  max_public_ports_per_service: 4
```

Common server flags mirror config keys: `--addr`, `--database-url`, `--log-level`, `--bootstrap-token`, `--jwt-secret`, `--users`, `--secret-key`, `--shutdown-timeout`, `--node-heartbeat-timeout`, and `--node-monitor-interval`.

Admission policy can also be supplied through `ORCH_POLICY_REQUIRE_RESOURCE_REQUESTS`, `ORCH_POLICY_REQUIRE_RESOURCE_LIMITS`, `ORCH_POLICY_ALLOW_PRIVILEGED`, `ORCH_POLICY_ALLOW_HOST_NETWORK`, `ORCH_POLICY_ALLOW_HOST_PID`, `ORCH_POLICY_ALLOWED_IMAGE_REGISTRIES`, `ORCH_POLICY_BLOCK_LATEST_TAG`, `ORCH_POLICY_REQUIRE_HEALTHCHECK`, `ORCH_POLICY_ALLOWED_HOST_PATH_PREFIXES`, `ORCH_POLICY_ALLOWED_CAPABILITIES`, `ORCH_POLICY_MAX_REPLICAS_PER_SERVICE`, and `ORCH_POLICY_MAX_PUBLIC_PORTS_PER_SERVICE`. List values are comma-separated.

## Agent

Run with a YAML config file:

```sh
orch-agent --config /etc/orch/agent.yaml
```

Print the effective agent config with secrets redacted:

```sh
orch-agent config print --config /etc/orch/agent.yaml
```

Example `agent.yaml`:

```yaml
node_name: worker-a
advertise_address: http://worker-a:8081
agent_addr: :8081
labels:
  role: worker
server_url: http://orch-server:8080
bootstrap_token: change-me
docker_socket_path: /var/run/docker.sock
log_level: info
heartbeat_interval: 5s
graceful_shutdown_ttl: 10s
```

Common agent flags mirror config keys: `--node-name`, `--advertise-address`, `--agent-addr`, `--labels`, `--server-url`, `--bootstrap-token`, `--docker-socket`, `--log-level`, `--heartbeat-interval`, and `--shutdown-timeout`.

## CLI

The CLI reads `--server`, `--token`, and `--namespace` (`-n`) flags, `ORCH_SERVER_URL`, `ORCH_TOKEN`, and `ORCH_NAMESPACE`, then its YAML config file. The default config path is the OS user config directory under `orch/config.yaml`. The namespace defaults to `default`.

Example CLI config:

```yaml
server_url: http://localhost:8080
token: eyJ...
namespace: default
```

## Redaction

`config print` redacts bootstrap tokens, JWT secrets, user maps, database URLs, and secret encryption keys. Do not put secret values in logs or support bundles except through redacted output.
