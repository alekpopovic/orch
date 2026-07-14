# 36. Traefik integration

```text
Add optional Traefik integration for HTTP routing.

Context:
For MVP, the orchestrator can use Traefik as an external reverse proxy instead of implementing its own proxy.

Task:
Create an optional integration that exposes service routing metadata for Traefik.

Support service YAML:

routes:
  - host: api.example.com
    pathPrefix: /
    port: 8080
    tls: true

Implementation options:
- Generate a dynamic Traefik file provider config, or
- Expose an HTTP provider endpoint that Traefik can poll.

Choose the simpler implementation for this repository.

Requirements:
- Routing config should include only healthy running tasks.
- Update config when tasks start/stop/become unhealthy.
- Add tests for generated routing config.
- Add docs/TRAEFIK.md with local dev example.
- Add docker-compose example with Traefik + demo service.
- Do not require Traefik for core orchestrator operation.
```
