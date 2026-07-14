# 06. Docker Engine runtime wrapper

```text
Implement a Docker Engine runtime package.

Package: internal/docker

Create an interface:

type Runtime interface {
    PullImage(ctx context.Context, image string, auth *RegistryAuth) error
    CreateContainer(ctx context.Context, spec ContainerSpec) (ContainerID, error)
    StartContainer(ctx context.Context, id ContainerID) error
    StopContainer(ctx context.Context, id ContainerID, timeout time.Duration) error
    RemoveContainer(ctx context.Context, id ContainerID, force bool) error
    InspectContainer(ctx context.Context, id ContainerID) (ContainerStatus, error)
    ListManagedContainers(ctx context.Context, labels map[string]string) ([]ContainerStatus, error)
    StreamLogs(ctx context.Context, id ContainerID, opts LogOptions) (<-chan LogLine, <-chan error)
}

Requirements:
- Use official Docker Go SDK.
- Every container created by this orchestrator must include labels:
  - orch.managed=true
  - orch.service_id
  - orch.task_id
  - orch.node_id
  - orch.version
- Operations must be idempotent:
  - creating an existing task should not create duplicate containers
  - stopping an already stopped container should be success
  - removing an already removed container should be success
- Support:
  - env vars
  - exposed ports
  - resource limits
  - restart policy disabled by default, because orchestrator controls restarts
  - healthcheck config if possible
- Add unit tests using mocks.
- Add integration tests guarded by an environment variable, e.g. ORCH_DOCKER_INTEGRATION=1.
- Document required Docker daemon permissions and security risks.
```
