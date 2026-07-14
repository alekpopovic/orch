# 45. Structured error model

```text
Implement a consistent structured error model across API, CLI, controllers, and agent.

Requirements:
- Create domain error types:
  - not_found
  - invalid_argument
  - conflict
  - unauthorized
  - forbidden
  - failed_precondition
  - unavailable
  - internal
- API errors should include:
  - code
  - message
  - request_id
  - details optional
- CLI should convert API errors into helpful human-readable messages.
- Controllers should emit structured events for important errors.
- Logs should include error code and object identifiers.
- Do not leak secrets in error details.

Add tests for:
- API error mapping
- CLI rendering
- redaction
- controller event emission

Update docs/API.md.
```
