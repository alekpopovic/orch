# 59. Chaos testing scenarios

```text
Add chaos-style integration tests for orchestrator reliability.

Scenarios:
1. Agent restarts during task start.
2. Server restarts during rollout.
3. Docker runtime fails to start container.
4. Image pull fails repeatedly.
5. Node goes offline during scale-up.
6. Node returns with stale containers.
7. Database write fails during scheduler assignment.
8. User deletes service during rollout.

Requirements:
- Use fake runtime and fake clock where possible.
- Avoid arbitrary sleeps.
- Use polling with clear timeouts.
- Each scenario must assert final safe state and emitted events.
- Add docs/CHAOS_TESTING.md with explanation.
```
