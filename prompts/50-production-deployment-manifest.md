# 50. Production deployment manifest

```text
Create production deployment examples for the orchestrator itself.

Task:
Add examples for deploying:
- orch-server
- orch-agent
- PostgreSQL dependency assumptions
- Prometheus scraping

Deliverables:
- deployments/production/docker-compose.yml
- deployments/production/systemd/orch-agent.service
- deployments/production/systemd/orch-server.service
- docs/PRODUCTION_DEPLOYMENT.md

Requirements:
- Include environment variables.
- Include volume mounts.
- Include Docker socket access warning for agent.
- Include TLS/auth placeholders.
- Include systemd restart policies.
- Include log location guidance.
- Include firewall/ports documentation.
- Do not hardcode secrets.
```
