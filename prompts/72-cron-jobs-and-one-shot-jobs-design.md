# 72. Cron jobs and one-shot jobs design

```text
Create a design document for jobs and cron jobs.

Context:
The orchestrator currently focuses on long-running services. Many production systems also need one-shot jobs and scheduled jobs.

Task:
Create docs/JOBS_DESIGN.md.

Cover:
- one-shot jobs
- cron jobs
- retries
- backoff
- parallelism
- completions
- deadlines
- logs
- events
- resource requests
- secrets
- placement
- failure modes
- API proposal
- CLI proposal
- database schema proposal
- interaction with scheduler and agent
- cleanup/TTL

Do not implement jobs yet.
```
