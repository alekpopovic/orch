# 25. Failure modes hardening

```text
Analyze and improve behavior for these failure modes:

- agent process restarts
- server process restarts
- Docker daemon restarts
- node goes offline
- node returns after being offline
- task is manually deleted using docker rm
- container exits with non-zero code
- image pull fails
- port is already allocated
- database temporarily unavailable
- rollout interrupted mid-way

For each failure mode:
1. Describe current behavior.
2. Identify unsafe behavior.
3. Implement the safest MVP fix.
4. Add tests.
5. Add docs/RELIABILITY.md section.

Do not over-engineer HA yet.
Focus on deterministic recovery and clear events.
```
