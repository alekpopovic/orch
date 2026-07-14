# 22. End-to-end tests

```text
Implement end-to-end tests for the orchestrator MVP.

Test scenario:
1. Start test PostgreSQL.
2. Start orch-server.
3. Start fake or real orch-agent.
4. Deploy service with 2 replicas.
5. Verify 2 tasks are created.
6. Verify scheduler assigns tasks.
7. Verify agent starts containers or fake runtime transitions them to running.
8. Scale service to 3 replicas.
9. Verify one additional task.
10. Scale service to 1 replica.
11. Verify extra tasks are stopped.
12. Delete service.
13. Verify cleanup.

Requirements:
- Prefer fake Docker runtime for stable CI.
- Add optional real Docker E2E behind ORCH_E2E_DOCKER=1.
- Tests must be deterministic.
- Avoid arbitrary sleeps; use polling with timeouts.
- Add CI workflow for unit tests.
```
