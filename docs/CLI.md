# CLI Reference

`orch` is the operator CLI for the v0.3.0 control-plane API. It deploys and inspects workloads, manages nodes and namespaces, performs rollouts, and exposes the operational safety features added in v0.3.0.

Install it with the [installation guide](https://alekpopovic.github.io/orch/#INSTALLATION.md), then confirm the binary version:

```sh
orch version
```

## Global Options

Global flags may appear before or after a command:

| Flag | Environment | Config key | Default | Purpose |
| --- | --- | --- | --- | --- |
| `--server` | `ORCH_SERVER_URL` | `server_url` | required | Control-plane URL. |
| `--token` | `ORCH_TOKEN` | `token` | empty | User API JWT bearer token. |
| `--namespace`, `-n` | `ORCH_NAMESPACE` | `namespace` | `default` | Workload namespace. |
| `--output` | — | — | `table` | Output format: `table` or `json`. |
| `--config` | — | — | OS user config path | YAML config file. |

Precedence is command flag, environment variable, YAML config, then default. The default config location is `orch/config.yaml` inside the operating system's user configuration directory (normally `~/.config/orch/config.yaml` on Linux).

The server URL has no built-in default. Commands that contact the API fail with an actionable error until it is supplied by flag, environment, or config.

Example:

```yaml
server_url: https://orch.example.com
token: eyJ...
namespace: payments
```

Avoid placing long-lived tokens in shell history. Prefer `ORCH_TOKEN` supplied by a secret-aware session or a config file with user-only permissions.

## First Connection

```sh
export ORCH_SERVER_URL=https://orch.example.com
export ORCH_TOKEN='<jwt>'

orch cluster check-upgrade
orch namespace ls
orch node ls
orch service ls --output json
```

`cluster check-upgrade` compares the server compatibility contract with registered agents. It reports a blocked state for agents older than v0.2.0 and warns about versions newer than the v0.3.0 tested maximum.

## Command Overview

| Command | Purpose |
| --- | --- |
| `orch version` | Print the CLI version. |
| `orch validate <file.yaml>` | Validate a service manifest locally. |
| `orch deploy <file.yaml>` | Create a service from YAML. |
| `orch service ls` | List services in the selected namespace. |
| `orch service inspect <service>` | Show one service by name or ID. |
| `orch service ps <service>` | List tasks for a service. |
| `orch scale <service> --replicas <n>` | Change desired replicas. |
| `orch rollout <service> --image <image>` | Roll out a new image. |
| `orch rollout status <service>` | Show the latest rollout. |
| `orch rollback <service>` | Roll back the service. |
| `orch delete <service>` | Delete a service. |
| `orch endpoints <service>` | List healthy discovery endpoints. |
| `orch logs <service>` | Fetch or follow service logs. |
| `orch events` | List or follow events. |
| `orch audit` | Query audit history. |
| `orch node ...` | Inspect, drain, and uncordon nodes. |
| `orch namespace ...` | Create, list, and delete namespaces. |
| `orch quota ...` | Inspect or set namespace quotas. |
| `orch gitops ...` | Manage and synchronize GitOps sources. |
| `orch job ...` | Run and inspect finite jobs. |
| `orch cronjob ...` | Manage scheduled jobs. |
| `orch volume ...` | Manage local persistent-volume claims. |
| `orch maintenance ...` | Manage operation windows. |
| `orch retention ...` | Inspect or execute retention pruning. |
| `orch usage ...` | Inspect or export namespace usage. |
| `orch cluster check-upgrade` | Check server, agent, API, and schema compatibility. |

Run `orch help`, `orch <command> --help`, or `orch completion --help` for generated command help.

## Services And Rollouts

Validate before deployment, then inspect the resulting service and tasks:

```sh
orch validate deployments/examples/http-api.yaml
orch --namespace default deploy deployments/examples/http-api.yaml
orch service inspect http-api
orch service ps http-api
```

Operate a service:

```sh
orch scale http-api --replicas 3
orch rollout http-api --image nginx:1.28-alpine \
  --max-unavailable 1 --max-surge 1
orch rollout status http-api
orch rollback http-api
orch delete http-api
```

Scale, rollout, rollback, and node drain may be restricted to a maintenance window. `--force` bypasses the restriction for authorized emergency work and records the override in the audit log.

## Logs, Events, Endpoints, And Audit

```sh
orch endpoints http-api
orch endpoints http-api --include-unhealthy
orch logs http-api --tail 100
orch logs http-api --task <task-id> --follow
orch events --service http-api --severity error
orch events --type rollout.status.changed --follow
orch audit --actor-type user --outcome failure --limit 50
orch audit --since 2026-08-21T00:00:00Z --output json
```

Use `Ctrl-C` to stop followed log or event streams.

## Nodes

```sh
orch node ls
orch node inspect <node-id>
orch node drain <node-id>
orch node drain-status <node-id>
orch node uncordon <node-id>
```

For emergency drain outside an allowed maintenance window, authorized operators may add `--force`.

## Namespaces And Quotas

```sh
orch namespace create payments
orch namespace ls
orch --namespace payments quota get
orch --namespace payments quota set \
  --max-services 20 \
  --max-replicas 100 \
  --max-cpu-millicores 20000 \
  --max-memory-bytes 68719476736 \
  --max-public-ports 10 \
  --max-secrets 50 \
  --max-registry-credentials 10
orch namespace delete payments
```

Zero quota values mean unlimited. Namespace deletion succeeds only when the namespace is empty.

## GitOps

```sh
orch --namespace payments gitops add \
  --repo https://github.com/acme/orch-config.git \
  --branch main \
  --path services \
  --interval 1m \
  --prune \
  --drift-policy warn
orch gitops ls
orch gitops sync <source-id>
orch gitops status
orch gitops diff <service>
orch gitops delete <source-id>
```

`--drift-policy` accepts `warn` or `auto_revert`. See [GitOps](https://alekpopovic.github.io/orch/#GITOPS.md) for credential and reconciliation boundaries.

## Jobs And Cron Jobs

```sh
orch job run deployments/examples/worker.yaml
orch job ls
orch job logs <job-id>

orch cronjob apply <cronjob.yaml>
orch cronjob ls
orch cronjob suspend <cronjob-id>
orch cronjob resume <cronjob-id>
```

Job and cron job manifests are documented in [Jobs](https://alekpopovic.github.io/orch/#JOBS.md).

## Volumes

```sh
orch volume create app-data --node <node-id>
orch volume ls
orch volume inspect <volume-id>
```

v0.3.0 volumes use the local driver. A node pin makes the locality constraint explicit. Review [Storage](https://alekpopovic.github.io/orch/#STORAGE.md) before relying on them.

## Maintenance, Retention, And Usage

```sh
orch maintenance create weekly-ops \
  --schedule '0 2 * * 0' \
  --timezone Europe/Belgrade \
  --duration 2h \
  --operations rollout,rollback,node_drain
orch maintenance ls
orch maintenance delete <window-id>

orch retention status
orch retention prune --dry-run
orch retention prune

orch --namespace payments usage --from 2026-08-01 --to 2026-09-01
orch --namespace payments usage export \
  --from 2026-08-01 --to 2026-09-01 --format csv > usage.csv
```

Run retention with `--dry-run` first. Dates accept `YYYY-MM-DD` or RFC3339 timestamps; API timestamps and reports are UTC.

## Automation

Use JSON output where supported and pin the CLI version in CI:

```sh
orch --server "$ORCH_SERVER_URL" \
  --token "$ORCH_TOKEN" \
  --namespace payments \
  --output json service ls
```

The CLI returns a non-zero exit status on validation, authentication, authorization, compatibility, and API failures. Never enable `--force` as an unconditional automation default.

## Related Documentation

- [Installation](https://alekpopovic.github.io/orch/#INSTALLATION.md)
- [Configuration](https://alekpopovic.github.io/orch/#CONFIGURATION.md)
- [Service spec](https://alekpopovic.github.io/orch/#SERVICE_SPEC.md)
- [Cluster upgrades](https://alekpopovic.github.io/orch/#UPGRADES.md)
- [API](https://alekpopovic.github.io/orch/#API.md)
- [v0.3.0 release notes](https://alekpopovic.github.io/orch/#RELEASE_NOTES.md)
