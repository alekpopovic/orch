# 24. Idempotency audit

```text
Perform an idempotency audit of the orchestrator.

Review:
- API handlers
- reconciler
- scheduler
- agent task execution
- Docker runtime wrapper
- rollout controller
- service deletion

Find cases where retrying the same operation could:
- create duplicate tasks
- create duplicate containers
- assign a task twice
- stop the wrong task
- corrupt rollout state
- emit misleading status
- leak resources

Then implement fixes.

Requirements:
- Add tests proving idempotency for critical flows.
- Add comments only where they clarify non-obvious behavior.
- Update docs/RELIABILITY.md with idempotency rules.
```
