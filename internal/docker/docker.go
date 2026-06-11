package docker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	ManagedLabel   = "orch.managed"
	ServiceIDLabel = "orch.service_id"
	TaskIDLabel    = "orch.task_id"
	NodeIDLabel    = "orch.node_id"
	VersionLabel   = "orch.version"
)

type ContainerID string

type RegistryAuth struct {
	Username      string
	Password      string
	Auth          string
	ServerAddress string
	IdentityToken string
}

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

type ContainerSpec struct {
	Name        string
	Image       string
	ServiceID   string
	TaskID      string
	NodeID      string
	Version     int64
	Env         map[string]string
	Ports       []PortBinding
	Resources   ResourceLimits
	Healthcheck *Healthcheck
	Labels      map[string]string
	Command     []string
}

type PortBinding struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}

type ResourceLimits struct {
	CPUMilli int64
	Memory   int64
}

type Healthcheck struct {
	Test        []string
	Interval    time.Duration
	Timeout     time.Duration
	StartPeriod time.Duration
	Retries     int
}

type ContainerStatus struct {
	ID         ContainerID
	Name       string
	Image      string
	State      string
	Status     string
	Running    bool
	ExitCode   int
	Labels     map[string]string
	CreatedAt  time.Time
	StartedAt  time.Time
	FinishedAt time.Time
}

type LogOptions struct {
	Follow     bool
	Tail       string
	Since      string
	Until      string
	Timestamps bool
	Stdout     bool
	Stderr     bool
}

type LogLine struct {
	Stream    string
	Line      string
	Timestamp time.Time
}

type EngineRuntime struct {
	client dockerClient
}

type dockerClient interface {
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerInspect(ctx context.Context, containerID string) (dockertypes.ContainerJSON, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]dockertypes.Container, error)
	ContainerLogs(ctx context.Context, container string, options container.LogsOptions) (io.ReadCloser, error)
}

func NewEngineRuntime(cli *client.Client) *EngineRuntime {
	return &EngineRuntime{client: cli}
}

func NewEngineRuntimeFromEnv() (*EngineRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return NewEngineRuntime(cli), nil
}

func NewEngineRuntimeWithClient(cli dockerClient) *EngineRuntime {
	return &EngineRuntime{client: cli}
}

func (r *EngineRuntime) PullImage(ctx context.Context, imageName string, auth *RegistryAuth) error {
	if strings.TrimSpace(imageName) == "" {
		return fmt.Errorf("image is required")
	}
	reader, err := r.client.ImagePull(ctx, imageName, image.PullOptions{RegistryAuth: encodeRegistryAuth(auth)})
	if err != nil {
		return fmt.Errorf("pull image %q: %w", imageName, err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func (r *EngineRuntime) CreateContainer(ctx context.Context, spec ContainerSpec) (ContainerID, error) {
	if err := spec.Validate(); err != nil {
		return "", err
	}

	existing, err := r.findContainerForTask(ctx, spec.TaskID)
	if err == nil {
		return existing.ID, nil
	}
	if err != nil && !errdefs.IsNotFound(err) {
		return "", err
	}

	config, hostConfig, err := dockerContainerConfig(spec)
	if err != nil {
		return "", err
	}
	response, err := r.client.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, spec.Name)
	if err != nil {
		if errdefs.IsConflict(err) {
			existing, findErr := r.findContainerForTask(ctx, spec.TaskID)
			if findErr == nil {
				return existing.ID, nil
			}
		}
		return "", fmt.Errorf("create container for task %s: %w", spec.TaskID, err)
	}
	return ContainerID(response.ID), nil
}

func (r *EngineRuntime) StartContainer(ctx context.Context, id ContainerID) error {
	if id == "" {
		return fmt.Errorf("container id is required")
	}
	if err := r.client.ContainerStart(ctx, string(id), container.StartOptions{}); err != nil {
		if errdefs.IsNotModified(err) {
			return nil
		}
		return fmt.Errorf("start container %s: %w", id, err)
	}
	return nil
}

func (r *EngineRuntime) StopContainer(ctx context.Context, id ContainerID, timeout time.Duration) error {
	if id == "" {
		return fmt.Errorf("container id is required")
	}
	status, err := r.InspectContainer(ctx, id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !status.Running {
		return nil
	}

	seconds := int(timeout.Seconds())
	if timeout > 0 && seconds == 0 {
		seconds = 1
	}
	if err := r.client.ContainerStop(ctx, string(id), container.StopOptions{Timeout: &seconds}); err != nil {
		if errdefs.IsNotFound(err) || errdefs.IsNotModified(err) {
			return nil
		}
		return fmt.Errorf("stop container %s: %w", id, err)
	}
	return nil
}

func (r *EngineRuntime) RemoveContainer(ctx context.Context, id ContainerID, force bool) error {
	if id == "" {
		return fmt.Errorf("container id is required")
	}
	if err := r.client.ContainerRemove(ctx, string(id), container.RemoveOptions{Force: force, RemoveVolumes: true}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("remove container %s: %w", id, err)
	}
	return nil
}

func (r *EngineRuntime) InspectContainer(ctx context.Context, id ContainerID) (ContainerStatus, error) {
	if id == "" {
		return ContainerStatus{}, fmt.Errorf("container id is required")
	}
	inspect, err := r.client.ContainerInspect(ctx, string(id))
	if err != nil {
		return ContainerStatus{}, fmt.Errorf("inspect container %s: %w", id, err)
	}
	return statusFromInspect(inspect), nil
}

func (r *EngineRuntime) ListManagedContainers(ctx context.Context, labels map[string]string) ([]ContainerStatus, error) {
	args := filters.NewArgs(filters.Arg("label", ManagedLabel+"=true"))
	for key, value := range labels {
		args.Add("label", key+"="+value)
	}
	containers, err := r.client.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, fmt.Errorf("list managed containers: %w", err)
	}
	statuses := make([]ContainerStatus, 0, len(containers))
	for _, c := range containers {
		statuses = append(statuses, statusFromList(c))
	}
	return statuses, nil
}

func (r *EngineRuntime) StreamLogs(ctx context.Context, id ContainerID, opts LogOptions) (<-chan LogLine, <-chan error) {
	lines := make(chan LogLine)
	errs := make(chan error, 1)

	go func() {
		defer close(lines)
		defer close(errs)

		showStdout := opts.Stdout || (!opts.Stdout && !opts.Stderr)
		showStderr := opts.Stderr || (!opts.Stdout && !opts.Stderr)
		reader, err := r.client.ContainerLogs(ctx, string(id), container.LogsOptions{
			ShowStdout: showStdout,
			ShowStderr: showStderr,
			Follow:     opts.Follow,
			Tail:       defaultTail(opts.Tail),
			Since:      opts.Since,
			Until:      opts.Until,
			Timestamps: opts.Timestamps,
		})
		if err != nil {
			errs <- fmt.Errorf("stream logs for container %s: %w", id, err)
			return
		}
		defer reader.Close()

		stdoutReader, stdoutWriter := io.Pipe()
		stderrReader, stderrWriter := io.Pipe()
		copyErr := make(chan error, 1)
		go func() {
			_, err := stdcopy.StdCopy(stdoutWriter, stderrWriter, reader)
			_ = stdoutWriter.Close()
			_ = stderrWriter.Close()
			copyErr <- err
		}()

		readDone := make(chan error, 2)
		go scanLogStream(ctx, stdoutReader, "stdout", lines, readDone)
		go scanLogStream(ctx, stderrReader, "stderr", lines, readDone)

		var readErr error
		for range 2 {
			if err := <-readDone; err != nil && readErr == nil {
				readErr = err
			}
		}
		if err := <-copyErr; err != nil && readErr == nil {
			readErr = err
		}
		if readErr != nil && !errors.Is(readErr, context.Canceled) {
			errs <- readErr
		}
	}()

	return lines, errs
}

func (r *EngineRuntime) findContainerForTask(ctx context.Context, taskID string) (ContainerStatus, error) {
	statuses, err := r.ListManagedContainers(ctx, map[string]string{TaskIDLabel: taskID})
	if err != nil {
		return ContainerStatus{}, err
	}
	if len(statuses) == 0 {
		return ContainerStatus{}, errdefs.NotFound(fmt.Errorf("container for task %s not found", taskID))
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].CreatedAt.Before(statuses[j].CreatedAt)
	})
	return statuses[0], nil
}

func (spec ContainerSpec) Validate() error {
	if strings.TrimSpace(spec.Image) == "" {
		return fmt.Errorf("image is required")
	}
	if strings.TrimSpace(spec.ServiceID) == "" {
		return fmt.Errorf("service id is required")
	}
	if strings.TrimSpace(spec.TaskID) == "" {
		return fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(spec.NodeID) == "" {
		return fmt.Errorf("node id is required")
	}
	if spec.Version <= 0 {
		return fmt.Errorf("version must be positive")
	}
	for i, port := range spec.Ports {
		if port.ContainerPort < 1 || port.ContainerPort > 65535 {
			return fmt.Errorf("ports[%d]: container port must be between 1 and 65535", i)
		}
		if port.HostPort < 0 || port.HostPort > 65535 {
			return fmt.Errorf("ports[%d]: host port must be between 0 and 65535", i)
		}
	}
	return nil
}

func dockerContainerConfig(spec ContainerSpec) (*container.Config, *container.HostConfig, error) {
	labels := managedLabels(spec)
	env := envList(spec.Env)
	exposedPorts, portBindings, err := dockerPorts(spec.Ports)
	if err != nil {
		return nil, nil, err
	}

	config := &container.Config{
		Image:        spec.Image,
		Env:          env,
		Labels:       labels,
		ExposedPorts: exposedPorts,
		Cmd:          spec.Command,
		Healthcheck:  dockerHealthcheck(spec.Healthcheck),
	}
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		RestartPolicy: container.RestartPolicy{
			Name: "no",
		},
		Resources: container.Resources{
			Memory:   spec.Resources.Memory,
			NanoCPUs: spec.Resources.CPUMilli * 1_000_000,
		},
	}
	return config, hostConfig, nil
}

func managedLabels(spec ContainerSpec) map[string]string {
	labels := make(map[string]string, len(spec.Labels)+5)
	for key, value := range spec.Labels {
		labels[key] = value
	}
	labels[ManagedLabel] = "true"
	labels[ServiceIDLabel] = spec.ServiceID
	labels[TaskIDLabel] = spec.TaskID
	labels[NodeIDLabel] = spec.NodeID
	labels[VersionLabel] = strconv.FormatInt(spec.Version, 10)
	return labels
}

func envList(env map[string]string) []string {
	values := make([]string, 0, len(env))
	for key, value := range env {
		values = append(values, key+"="+value)
	}
	sort.Strings(values)
	return values
}

func dockerPorts(ports []PortBinding) (nat.PortSet, nat.PortMap, error) {
	exposed := nat.PortSet{}
	bindings := nat.PortMap{}
	for _, binding := range ports {
		protocol := strings.ToLower(strings.TrimSpace(binding.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		port := nat.Port(strconv.Itoa(binding.ContainerPort) + "/" + protocol)
		exposed[port] = struct{}{}
		if binding.HostPort > 0 {
			bindings[port] = append(bindings[port], nat.PortBinding{HostPort: strconv.Itoa(binding.HostPort)})
		}
	}
	return exposed, bindings, nil
}

func dockerHealthcheck(check *Healthcheck) *container.HealthConfig {
	if check == nil {
		return nil
	}
	return &container.HealthConfig{
		Test:        check.Test,
		Interval:    check.Interval,
		Timeout:     check.Timeout,
		StartPeriod: check.StartPeriod,
		Retries:     check.Retries,
	}
}

func statusFromInspect(inspect dockertypes.ContainerJSON) ContainerStatus {
	status := ContainerStatus{
		ID:     ContainerID(inspect.ID),
		Name:   strings.TrimPrefix(inspect.Name, "/"),
		Labels: map[string]string{},
	}
	if inspect.Config != nil {
		status.Image = inspect.Config.Image
		status.Labels = inspect.Config.Labels
	}
	if inspect.State != nil {
		status.State = inspect.State.Status
		status.Running = inspect.State.Running
		status.ExitCode = inspect.State.ExitCode
		status.StartedAt = parseDockerTime(inspect.State.StartedAt)
		status.FinishedAt = parseDockerTime(inspect.State.FinishedAt)
	}
	status.CreatedAt = parseDockerTime(inspect.Created)
	return status
}

func statusFromList(c dockertypes.Container) ContainerStatus {
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}
	return ContainerStatus{
		ID:        ContainerID(c.ID),
		Name:      name,
		Image:     c.Image,
		State:     c.State,
		Status:    c.Status,
		Labels:    c.Labels,
		CreatedAt: time.Unix(c.Created, 0).UTC(),
		Running:   c.State == "running",
	}
}

func parseDockerTime(value string) time.Time {
	if value == "" || strings.HasPrefix(value, "0001-01-01") {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func encodeRegistryAuth(auth *RegistryAuth) string {
	if auth == nil {
		return ""
	}
	config := registry.AuthConfig{
		Username:      auth.Username,
		Password:      auth.Password,
		Auth:          auth.Auth,
		ServerAddress: auth.ServerAddress,
		IdentityToken: auth.IdentityToken,
	}
	data, err := json.Marshal(config)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

func defaultTail(tail string) string {
	if strings.TrimSpace(tail) == "" {
		return "all"
	}
	return tail
}

func scanLogStream(ctx context.Context, reader io.Reader, stream string, out chan<- LogLine, done chan<- error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			done <- ctx.Err()
			return
		case out <- LogLine{Stream: stream, Line: scanner.Text(), Timestamp: time.Now().UTC()}:
		}
	}
	done <- scanner.Err()
}
