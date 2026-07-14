# 86. Fuzz tests for parsers and validators

```text
Add fuzz tests for high-risk parsers and validators.

Targets:
- service YAML parser
- resource quantity parser
- cron expression parser if implemented
- image reference parser
- port spec parser
- label selector parser
- healthcheck path validator
- OpenAPI request validation if applicable

Requirements:
- Use Go fuzzing support.
- Fuzz tests must not require Docker or network.
- Add seed corpus with valid and invalid examples.
- Ensure fuzz failures produce useful minimized cases.
- Add docs/TESTING.md explaining how to run fuzz tests.
- Add CI job only if runtime is reasonable; otherwise document manual command.

At the end:
- Run normal go test ./...
```
