# 43. Deployment YAML schema and validation

```text
Create a formal deployment YAML schema and validation system.

Context:
The CLI accepts service YAML files. Validation must catch errors before sending to the server.

Task:
Define the canonical service spec schema.

Requirements:
- Add docs/SERVICE_SPEC.md.
- Add JSON Schema or equivalent schema file under schemas/service.schema.json.
- CLI must validate:
  - required name
  - valid image
  - replicas >= 0
  - valid ports
  - valid resources
  - valid healthcheck
  - valid placement labels
  - valid routes
  - valid secret references
- Server must repeat validation and not trust CLI.
- Add command:
  orch validate <file.yaml>
- Add tests with valid and invalid YAML examples.
- Add examples:
  - simple web service
  - worker
  - private image
  - service with secrets
  - service with route
```
