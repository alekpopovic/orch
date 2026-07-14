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
```

Common server flags mirror config keys: `--addr`, `--database-url`, `--log-level`, `--bootstrap-token`, `--jwt-secret`, `--users`, `--secret-key`, `--shutdown-timeout`, `--node-heartbeat-timeout`, and `--node-monitor-interval`.

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

The CLI reads `--server` and `--token` flags, `ORCH_SERVER_URL` and `ORCH_TOKEN`, then its YAML config file. The default config path is the OS user config directory under `orch/config.yaml`.

Example CLI config:

```yaml
server_url: http://localhost:8080
token: eyJ...
```

## Redaction

`config print` redacts bootstrap tokens, JWT secrets, user maps, database URLs, and secret encryption keys. Do not put secret values in logs or support bundles except through redacted output.
