# 74. Cron jobs MVP

```text
Implement cron jobs MVP.

Context:
One-shot jobs exist. Now add scheduled jobs.

Task:
Add CronJob model and controller.

YAML example:

kind: CronJob
name: nightly-report
schedule: "0 2 * * *"
concurrencyPolicy: Forbid
jobTemplate:
  image: ghcr.io/example/reporter:1.0.0
  command: ["./report"]
  resources:
    cpu: 250m
    memory: 256Mi

Requirements:
- Parse cron schedules.
- Support timezone explicitly; default UTC.
- Support concurrency policies:
  - Allow
  - Forbid
  - Replace
- Create Job instances on schedule.
- Add missed schedule handling with a configurable limit.
- Add API:
  - POST /v1/cronjobs
  - GET /v1/cronjobs
  - GET /v1/cronjobs/{id}
  - DELETE /v1/cronjobs/{id}
- Add CLI:
  - orch cronjob apply <file.yaml>
  - orch cronjob ls
  - orch cronjob suspend <name>
  - orch cronjob resume <name>
- Add tests using fake clock.
- Update docs/JOBS.md.

At the end:
- Run go test ./...
```
