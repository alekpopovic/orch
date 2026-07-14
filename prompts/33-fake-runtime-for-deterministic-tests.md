# 33. Fake runtime for deterministic tests

```text
Implement a fake container runtime for deterministic unit and E2E tests.

Context:
The orchestrator uses Docker Engine in production, but CI and controller tests need a fake runtime.

Task:
Create internal/runtime abstraction if not already present, then add:
- DockerRuntime implementation
- FakeRuntime implementation

FakeRuntime should support:
- PullImage
- CreateContainer
- StartContainer
- StopContainer
- RemoveContainer
- InspectContainer
- ListManagedContainers
- StreamLogs
- configurable failures:
  - image pull failure
  - create failure
  - start failure
  - health failure
  - container exits after start

Requirements:
- FakeRuntime must be thread-safe.
- FakeRuntime must record operations for assertions.
- Tests must not depend on real Docker unless explicitly enabled.
- Existing agent tests should use FakeRuntime.
- Add examples of failure injection tests.
- Document how to run real Docker integration tests separately.

At the end:
- Run go test ./...
```
