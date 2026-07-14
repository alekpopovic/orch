# 48. Grafana dashboard JSON

```text
Create a starter Grafana dashboard for the orchestrator.

Task:
Add deploy/monitoring/grafana/orchestrator-dashboard.json.

Dashboard panels:
- API request rate
- API error rate
- API latency
- scheduler runs/errors
- reconciler runs/errors
- task state counts
- node status counts
- agent heartbeat failures
- Docker operation errors
- rollout success/failure counts
- healthcheck success/failure

Requirements:
- Use Prometheus datasource variable.
- Keep dashboard generic.
- Add docs/OBSERVABILITY.md instructions.
```
