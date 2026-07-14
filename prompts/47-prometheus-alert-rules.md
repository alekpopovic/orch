# 47. Prometheus alert rules

```text
Create recommended Prometheus alert rules for the orchestrator.

Task:
Add deploy/monitoring/prometheus-rules.yaml.

Alerts:
- OrchServerDown
- OrchAgentHeartbeatMissing
- NodeOffline
- SchedulerErrorsHigh
- ReconcilerErrorsHigh
- RolloutFailed
- TaskFailuresHigh
- DockerOperationErrorsHigh
- DatabaseUnavailable
- APIErrorRateHigh
- ReconciliationLatencyHigh

Requirements:
- Use practical labels and annotations.
- Keep labels low-cardinality.
- Add docs/OBSERVABILITY.md section explaining alerts.
- Add local docker-compose monitoring example if Prometheus exists.
- Do not require full Grafana setup yet.
```
