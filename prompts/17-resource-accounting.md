# 17. Resource accounting

```text
Implement resource accounting.

Goal:
Scheduler should avoid overcommitting CPU and memory based on service resource requests.

Requirements:
- Parse CPU values:
  - "500m"
  - "1"
  - "2.5"
- Parse memory values:
  - "128Mi"
  - "512Mi"
  - "1Gi"
  - "2G"
- Store normalized values:
  - CPU in millicores
  - memory in bytes
- Add validation:
  - requests cannot exceed limits
  - values must be positive
  - missing values should use configurable defaults
- Scheduler should compute allocated resources from non-terminal tasks.
- Add tests for parsers and scheduler accounting.
- Add documentation.
```
