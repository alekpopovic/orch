# Jobs

Run finite work with `orch job run job.yaml`, inspect it with `orch job ls`, and stream its latest attempt with `orch job logs <job-id>`. Jobs retry non-zero exits through `backoffLimit` and finish as `succeeded` or `failed`.

CronJobs use five-field cron syntax, an explicit IANA timezone (UTC by default), and `Allow`, `Forbid`, or `Replace` concurrency. Use `orch cronjob apply`, `ls`, `suspend`, and `resume`. Missed occurrences are bounded. See [JOBS_DESIGN.md](JOBS_DESIGN.md) for lifecycle and failure semantics.
