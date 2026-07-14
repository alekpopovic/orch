# Production Deployment

This guide provides production-oriented examples for running `orch-server`, `orch-agent`, PostgreSQL, and Prometheus scraping. The examples are starting points, not a full managed production platform.

## Assumptions

- PostgreSQL is durable and backed up.
- `orch-server` runs behind TLS termination such as a load balancer, reverse proxy, or service mesh.
- JWT authentication is enabled for user-facing APIs.
- Agent registration tokens, JWT secrets, TLS keys, database passwords, and `ORCH_SECRET_KEY` are stored in a secret manager or root-readable env files.
- Agents run only on trusted nodes.

## Docker Socket Warning

`orch-agent` requires Docker Engine API access. Mounting `/var/run/docker.sock` gives the agent effective root control over the host. Treat agent compromise as node compromise.

Controls:

- run agents only on nodes dedicated to orchestration workloads;
- restrict inbound access to the agent HTTP port;
- use the `docker` group only for the agent identity;
- monitor Docker operation error metrics and audit logs;
- never mount the Docker socket into workload containers.

## Docker Compose Example

Example file: `deployments/production/docker-compose.yml`.

Required image variables:

```sh
export ORCH_SERVER_IMAGE=ghcr.io/example/orch-server:<version>
export ORCH_AGENT_IMAGE=ghcr.io/example/orch-agent:<version>
```

Required secret/config variables:

```sh
export POSTGRES_PASSWORD='<from-secret-manager>'
export DATABASE_URL='postgres://orch:<password>@postgres:5432/orch?sslmode=require'
export ORCH_AGENT_REGISTRATION_TOKEN='<from-secret-manager>'
export ORCH_JWT_SECRET='<from-secret-manager>'
export ORCH_SECRET_KEY='<from-secret-manager>'
export ORCH_SERVER_URL='https://orch.example.com'
export ORCH_NODE_NAME="$(hostname -f)"
export ORCH_ADVERTISE_ADDRESS='https://agent-node-1.example.com:8081'
```

Optional:

```sh
export ORCH_USERS='alice:admin,bob:operator,carol:viewer'
export ORCH_LOG_LEVEL=info
export ORCH_NODE_LABELS='role=worker,zone=az-a'
export ORCH_AGENT_HEARTBEAT_INTERVAL=5s
```

Start:

```sh
docker compose -f deployments/production/docker-compose.yml up -d
```

Start with monitoring:

```sh
docker compose -f deployments/production/docker-compose.yml --profile monitoring up -d
```

Volumes:

- `postgres-data`: PostgreSQL state;
- `prometheus-data`: Prometheus TSDB when monitoring profile is enabled;
- `./config/server` and `./config/agent`: read-only config placeholders;
- `./tls`: TLS material placeholder if future native TLS mode or local proxy configuration needs it;
- `/var/run/docker.sock`: agent Docker access.

## systemd Example

Unit files:

- `deployments/production/systemd/orch-server.service`
- `deployments/production/systemd/orch-agent.service`

Install binaries:

```sh
install -o root -g root -m 0755 orch-server /usr/local/bin/orch-server
install -o root -g root -m 0755 orch-agent /usr/local/bin/orch-agent
```

Create users:

```sh
useradd --system --home /var/lib/orch --shell /usr/sbin/nologin orch
useradd --system --home /var/lib/orch-agent --shell /usr/sbin/nologin orch-agent
usermod -aG docker orch-agent
```

Create server env file at `/etc/orch/orch-server.env`:

```sh
DATABASE_URL=postgres://orch:<password>@db.example.com:5432/orch?sslmode=require
ORCH_SERVER_ADDR=:8080
ORCH_AGENT_REGISTRATION_TOKEN=<from-secret-manager>
ORCH_JWT_SECRET=<from-secret-manager>
ORCH_SECRET_KEY=<from-secret-manager>
ORCH_USERS=alice:admin,bob:operator
ORCH_LOG_LEVEL=info
ORCH_NODE_HEARTBEAT_TIMEOUT=30s
ORCH_NODE_MONITOR_INTERVAL=5s
ORCH_SHUTDOWN_TIMEOUT=15s
```

Create agent env file at `/etc/orch/orch-agent.env`:

```sh
ORCH_SERVER_URL=https://orch.example.com
ORCH_AGENT_REGISTRATION_TOKEN=<from-secret-manager>
ORCH_NODE_NAME=node-1.example.com
ORCH_ADVERTISE_ADDRESS=https://node-1.example.com:8081
ORCH_AGENT_ADDR=:8081
ORCH_NODE_LABELS=role=worker,zone=az-a
ORCH_DOCKER_SOCKET=/var/run/docker.sock
ORCH_AGENT_HEARTBEAT_INTERVAL=5s
ORCH_LOG_LEVEL=info
ORCH_SHUTDOWN_TIMEOUT=15s
```

Protect env files:

```sh
chown root:orch /etc/orch/orch-server.env
chmod 0640 /etc/orch/orch-server.env
chown root:orch-agent /etc/orch/orch-agent.env
chmod 0640 /etc/orch/orch-agent.env
```

Install and start:

```sh
cp deployments/production/systemd/orch-server.service /etc/systemd/system/
cp deployments/production/systemd/orch-agent.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now orch-server
systemctl enable --now orch-agent
```

## PostgreSQL Dependency

Production PostgreSQL should provide:

- durable storage with backups and restore testing;
- TLS transport where possible (`sslmode=require` or stronger);
- least-privilege database user for `orch`;
- migration execution before deploying binaries that require new schema;
- monitoring through `postgres-exporter` or equivalent.

See [BACKUP_RESTORE.md](https://alekpopovic.github.io/orch/#BACKUP_RESTORE.md).

## Prometheus Scraping

Scrape targets:

```yaml
scrape_configs:
  - job_name: orch-server
    static_configs:
      - targets: ["orch-server.example.com:8080"]

  - job_name: orch-agent
    static_configs:
      - targets:
          - "node-1.example.com:8081"
          - "node-2.example.com:8081"

  - job_name: postgres
    static_configs:
      - targets: ["postgres-exporter.example.com:9187"]
```

Alert rules: `deploy/monitoring/prometheus-rules.yaml`.

Dashboard: `deploy/monitoring/grafana/orchestrator-dashboard.json`.

## TLS And Authentication

Recommended layout:

- terminate public TLS at a reverse proxy/load balancer;
- forward only authenticated user traffic to `orch-server`;
- require `Authorization: Bearer <jwt>` for user routes by setting `ORCH_JWT_SECRET`;
- keep `/metrics` on a private monitoring network;
- keep agent endpoints on a private node network;
- rotate `ORCH_AGENT_REGISTRATION_TOKEN` after node enrollment when possible.

The example manifests include TLS env placeholders but do not hardcode certificates or secrets.

## Firewall And Ports

| Port | Component | Direction | Notes |
| --- | --- | --- | --- |
| `8080` | `orch-server` | users/agents/Prometheus to server | Put behind TLS/auth proxy for user traffic. |
| `8081` | `orch-agent` | server/Prometheus to agent | Restrict to control-plane and monitoring networks. |
| `5432` | PostgreSQL | server/admin to DB | Do not expose publicly. |
| `9187` | postgres-exporter | Prometheus to exporter | Monitoring network only. |
| `9090` | Prometheus | operator to Prometheus | Private/VPN only. |
| `80/443` | reverse proxy | users/agents to proxy | Prefer `443`; redirect or close `80`. |

## Logs

Docker Compose:

- use `docker compose logs orch-server orch-agent`;
- configure the Docker logging driver or external collector for retention;
- avoid logging request bodies or secret-bearing env files.

systemd:

```sh
journalctl -u orch-server -f
journalctl -u orch-agent -f
```

Unit files set `SyslogIdentifier` and `LogsDirectory=orch`. If you configure file logging later, place logs under `/var/log/orch` with logrotate and strict permissions.

## Deployment Checklist

1. Build and publish pinned server/agent images or install pinned binaries.
2. Apply database migrations.
3. Configure secrets in env files or secret manager.
4. Start PostgreSQL and verify backups.
5. Start `orch-server`.
6. Start agents on trusted Docker nodes.
7. Verify health, metrics, audit logs, and alerts.
8. Restrict firewall rules to the ports above.
