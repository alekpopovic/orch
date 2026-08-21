# Jobs and cron jobs design

`Job` represents finite work and `CronJob` creates Jobs on a schedule. Services remain the abstraction for continuously running workloads. The MVP reuses the Task lifecycle, scheduler resource accounting, agent container execution, logs, secrets, placement, and events.

## One-shot execution

A Job owns an immutable template and one or more attempt Tasks. It moves `pending -> running -> succeeded|failed`. Exit zero succeeds; non-zero exits retry until `backoffLimit` is exhausted, using capped exponential backoff with jitter. Future `parallelism` controls simultaneous Tasks, `completions` counts required successes, and an active deadline stops all attempts.

Logs remain addressable by Job and attempt. Events cover creation, scheduling, start, retry, success, failure, deadline, and deletion. Resource requests, secrets, volume claims, and placement use the same validation boundaries as Services.

## Scheduling

CronJobs use five-field cron expressions and explicit IANA timezones (UTC by default). An injected clock makes decisions deterministic. `Allow` permits overlap, `Forbid` skips while an owned Job is active, and `Replace` deletes active Jobs before starting the new occurrence. Catch-up is bounded by `missedScheduleLimit`.

## API, persistence, and controllers

Jobs use `POST/GET /v1/jobs`, `GET/DELETE /v1/jobs/{id}` and `orch job run|ls|logs`. CronJobs use `POST/GET /v1/cronjobs`, `GET/DELETE /v1/cronjobs/{id}`, suspend/resume subresources and `orch cronjob apply|ls|suspend|resume`.

`jobs` stores namespace, immutable JSON spec, status, attempts, exit code, and timestamps. Tasks carry nullable `job_id`; the final schema should enforce exactly one owner (`service_id` xor `job_id`). `cron_jobs` stores its template and last/next schedule. For HA, schedule advancement and Job creation must share a transaction and uniqueness key `(cron_job_id, scheduled_at)`.

Agent loss is handled through node-loss recovery. Image, secret, volume, placement and admission errors are permanent until the spec changes; runtime exits are retryable within the limit. `ttlSecondsAfterFinished` is the proposed cleanup control; until then deletion is explicit and logs/events follow independent retention policies.
