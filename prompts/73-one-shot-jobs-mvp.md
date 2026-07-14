# 73. One-shot jobs MVP

```text
Implement one-shot jobs MVP based on docs/JOBS_DESIGN.md.

Task:
Add support for running finite jobs.

YAML example:

kind: Job
name: db-migrate
image: ghcr.io/example/migrator:1.2.0
command: ["./migrate", "up"]
restart:
  policy: on-failure
backoffLimit: 3
resources:
  cpu: 500m
  memory: 512Mi

Requirements:
- Add Job model and Task reuse or separate JobTask if cleaner.
- Scheduler can place job tasks.
- Agent can run job container and report exit code.
- Reconciler handles retries up to backoffLimit.
- Job status:
  - pending
  - running
  - succeeded
  - failed
- Add API:
  - POST /v1/jobs
  - GET /v1/jobs
  - GET /v1/jobs/{id}
  - DELETE /v1/jobs/{id}
- Add CLI:
  - orch job run <file.yaml>
  - orch job ls
  - orch job logs <job>
- Add tests for success, failure, retry, delete.
- Update docs/JOBS.md.

At the end:
- Run go test ./...
```
