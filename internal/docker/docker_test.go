package docker

import (
	"bytes"
	"context"
	"io"
	"slices"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestCreateContainerAddsRequiredLabels(t *testing.T) {
	fake := &fakeDockerClient{createID: "created"}
	runtime := NewEngineRuntimeWithClient(fake)

	id, err := runtime.CreateContainer(context.Background(), containerSpecFixture())
	if err != nil {
		t.Fatalf("create container: %v", err)
	}

	if id != "created" {
		t.Fatalf("expected created id, got %q", id)
	}
	labels := fake.createdConfig.Labels
	for key, want := range map[string]string{
		ManagedLabel:   "true",
		ServiceIDLabel: "service-1",
		TaskIDLabel:    "task-1",
		NodeIDLabel:    "node-1",
		VersionLabel:   "7",
	} {
		if got := labels[key]; got != want {
			t.Fatalf("expected label %s=%s, got %q", key, want, got)
		}
	}
	if fake.createdHostConfig.RestartPolicy.Name != "no" {
		t.Fatalf("expected restart policy disabled, got %q", fake.createdHostConfig.RestartPolicy.Name)
	}
	if fake.createdHostConfig.Resources.NanoCPUs != 500_000_000 {
		t.Fatalf("unexpected cpu limit %d", fake.createdHostConfig.Resources.NanoCPUs)
	}
	if fake.createdHostConfig.Resources.Memory != 512*1024*1024 {
		t.Fatalf("unexpected memory limit %d", fake.createdHostConfig.Resources.Memory)
	}
	if len(fake.createdConfig.Env) != 2 || !slices.Contains(fake.createdConfig.Env, "A=1") {
		t.Fatalf("unexpected env %#v", fake.createdConfig.Env)
	}
	if !slices.Contains(fake.createdHostConfig.CapDrop, "NET_RAW") {
		t.Fatalf("expected NET_RAW dropped by default, got %#v", fake.createdHostConfig.CapDrop)
	}
}

func TestCreateContainerAppliesSecurityContext(t *testing.T) {
	fake := &fakeDockerClient{createID: "created"}
	runtime := NewEngineRuntimeWithClient(fake)
	spec := containerSpecFixture()
	spec.Security = SecurityContext{
		User:                   "1000:1000",
		ReadOnlyRootFilesystem: true,
		CapDrop:                []string{"ALL"},
		CapAdd:                 []string{"NET_BIND_SERVICE"},
		HostPathMounts: []HostPathMount{{
			HostPath:      "/var/lib/orch/service-a",
			ContainerPath: "/data",
			ReadOnly:      true,
		}},
	}

	if _, err := runtime.CreateContainer(context.Background(), spec); err != nil {
		t.Fatalf("create container: %v", err)
	}

	if fake.createdConfig.User != "1000:1000" {
		t.Fatalf("expected container user, got %q", fake.createdConfig.User)
	}
	if !fake.createdHostConfig.ReadonlyRootfs {
		t.Fatalf("expected read-only root filesystem")
	}
	if !slices.Contains(fake.createdHostConfig.CapDrop, "ALL") || !slices.Contains(fake.createdHostConfig.CapAdd, "NET_BIND_SERVICE") {
		t.Fatalf("unexpected capabilities drop=%#v add=%#v", fake.createdHostConfig.CapDrop, fake.createdHostConfig.CapAdd)
	}
	if len(fake.createdHostConfig.Binds) != 1 || fake.createdHostConfig.Binds[0] != "/var/lib/orch/service-a:/data:ro" {
		t.Fatalf("unexpected host path binds %#v", fake.createdHostConfig.Binds)
	}
}

func TestCreateContainerExistingTaskIsIdempotent(t *testing.T) {
	fake := &fakeDockerClient{
		listContainers: []dockertypes.Container{{
			ID:      "existing",
			Names:   []string{"/orch-task-1"},
			Labels:  map[string]string{ManagedLabel: "true", TaskIDLabel: "task-1"},
			Created: 10,
		}},
		createID: "created",
	}
	runtime := NewEngineRuntimeWithClient(fake)

	id, err := runtime.CreateContainer(context.Background(), containerSpecFixture())
	if err != nil {
		t.Fatalf("create container: %v", err)
	}

	if id != "existing" {
		t.Fatalf("expected existing container id, got %q", id)
	}
	if fake.createCalls != 0 {
		t.Fatalf("expected create not to be called, got %d calls", fake.createCalls)
	}
}

func TestStopContainerAlreadyStoppedIsSuccess(t *testing.T) {
	fake := &fakeDockerClient{
		inspect: dockertypes.ContainerJSON{
			ContainerJSONBase: &dockertypes.ContainerJSONBase{
				ID:    "stopped",
				State: &dockertypes.ContainerState{Status: "exited", Running: false},
			},
		},
	}
	runtime := NewEngineRuntimeWithClient(fake)

	if err := runtime.StopContainer(context.Background(), "stopped", 5*time.Second); err != nil {
		t.Fatalf("stop container: %v", err)
	}
	if fake.stopCalls != 0 {
		t.Fatalf("expected stop not to be called, got %d calls", fake.stopCalls)
	}
}

func TestRemoveContainerAlreadyRemovedIsSuccess(t *testing.T) {
	fake := &fakeDockerClient{removeErr: errdefs.NotFound(io.EOF)}
	runtime := NewEngineRuntimeWithClient(fake)

	if err := runtime.RemoveContainer(context.Background(), "missing", true); err != nil {
		t.Fatalf("remove missing container: %v", err)
	}
}

func TestListManagedContainersAddsManagedFilter(t *testing.T) {
	fake := &fakeDockerClient{}
	runtime := NewEngineRuntimeWithClient(fake)

	_, err := runtime.ListManagedContainers(context.Background(), map[string]string{ServiceIDLabel: "service-1"})
	if err != nil {
		t.Fatalf("list managed containers: %v", err)
	}

	if !fake.listOptions.Filters.ExactMatch("label", ManagedLabel+"=true") {
		t.Fatalf("expected managed label filter, got %#v", fake.listOptions.Filters)
	}
	if !fake.listOptions.Filters.ExactMatch("label", ServiceIDLabel+"=service-1") {
		t.Fatalf("expected service label filter, got %#v", fake.listOptions.Filters)
	}
}

func TestPullImageEncodesRegistryAuth(t *testing.T) {
	fake := &fakeDockerClient{}
	runtime := NewEngineRuntimeWithClient(fake)

	err := runtime.PullImage(context.Background(), "example.com/app:1", &RegistryAuth{
		Username:      "user",
		Password:      "pass",
		ServerAddress: "example.com",
	})
	if err != nil {
		t.Fatalf("pull image: %v", err)
	}
	if fake.pullOptions.RegistryAuth == "" {
		t.Fatalf("expected registry auth to be encoded")
	}
}

func TestContainerSpecValidation(t *testing.T) {
	spec := containerSpecFixture()
	spec.TaskID = ""

	if err := spec.Validate(); err == nil {
		t.Fatalf("expected missing task id to fail validation")
	}
}

func containerSpecFixture() ContainerSpec {
	return ContainerSpec{
		Name:      "orch-task-1",
		Image:     "nginx:latest",
		ServiceID: "service-1",
		TaskID:    "task-1",
		NodeID:    "node-1",
		Version:   7,
		Env: map[string]string{
			"A": "1",
			"B": "2",
		},
		Ports: []PortBinding{
			{ContainerPort: 8080, HostPort: 18080, Protocol: "tcp"},
		},
		Resources: ResourceLimits{
			CPUMilli: 500,
			Memory:   512 * 1024 * 1024,
		},
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD-SHELL", "curl -f http://localhost:8080/healthz"},
			Interval: 10 * time.Second,
			Timeout:  2 * time.Second,
			Retries:  3,
		},
		Labels: map[string]string{"custom": "label"},
	}
}

type fakeDockerClient struct {
	pullOptions       image.PullOptions
	createID          string
	createCalls       int
	createdConfig     *container.Config
	createdHostConfig *container.HostConfig
	startCalls        int
	stopCalls         int
	removeErr         error
	inspect           dockertypes.ContainerJSON
	listOptions       container.ListOptions
	listContainers    []dockertypes.Container
	logReader         io.ReadCloser
}

func (f *fakeDockerClient) ImagePull(_ context.Context, _ string, options image.PullOptions) (io.ReadCloser, error) {
	f.pullOptions = options
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (f *fakeDockerClient) ContainerCreate(_ context.Context, config *container.Config, hostConfig *container.HostConfig, _ *network.NetworkingConfig, _ *ocispec.Platform, _ string) (container.CreateResponse, error) {
	f.createCalls++
	f.createdConfig = config
	f.createdHostConfig = hostConfig
	return container.CreateResponse{ID: f.createID}, nil
}

func (f *fakeDockerClient) ContainerStart(context.Context, string, container.StartOptions) error {
	f.startCalls++
	return nil
}

func (f *fakeDockerClient) ContainerStop(context.Context, string, container.StopOptions) error {
	f.stopCalls++
	return nil
}

func (f *fakeDockerClient) ContainerRemove(context.Context, string, container.RemoveOptions) error {
	return f.removeErr
}

func (f *fakeDockerClient) ContainerInspect(context.Context, string) (dockertypes.ContainerJSON, error) {
	return f.inspect, nil
}

func (f *fakeDockerClient) ContainerList(_ context.Context, options container.ListOptions) ([]dockertypes.Container, error) {
	f.listOptions = options
	return f.listContainers, nil
}

func (f *fakeDockerClient) ContainerLogs(context.Context, string, container.LogsOptions) (io.ReadCloser, error) {
	if f.logReader != nil {
		return f.logReader, nil
	}
	return io.NopCloser(bytes.NewReader(nil)), nil
}

var _ dockerClient = (*fakeDockerClient)(nil)
