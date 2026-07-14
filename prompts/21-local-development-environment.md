# 21. Local development environment

```text
Create a complete local development environment.

Requirements:
- docker-compose.yml should start:
  - PostgreSQL
  - orch-server
  - one orch-agent
  - optional demo app
- Add scripts:
  - scripts/dev-up.sh
  - scripts/dev-down.sh
  - scripts/migrate-up.sh
  - scripts/migrate-down.sh
  - scripts/demo-deploy.sh
- Add example service YAMLs:
  - deployments/examples/http-api.yaml
  - deployments/examples/worker.yaml
- README quickstart should allow a developer to:
  1. start local environment
  2. run migrations
  3. register an agent
  4. deploy demo app
  5. scale demo app
  6. view logs/events
  7. delete demo app

Requirements:
- Scripts must fail fast.
- Scripts must be portable Bash.
- Do not require root except Docker access.
- Document prerequisites.
```
