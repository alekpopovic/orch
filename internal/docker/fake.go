package docker

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrFakeContainerNotFound = errors.New("fake container not found")

type FakeRuntime struct {
	mu sync.Mutex

	next               int
	nextIDs            []ContainerID
	containers         map[ContainerID]ContainerStatus
	byTask             map[string]ContainerID
	logs               map[ContainerID][]LogLine
	operations         []FakeOperation
	createdSpecs       []ContainerSpec
	pullErr            error
	pullAuths          []*RegistryAuth
	createErr          error
	startErr           error
	inspectErr         error
	listErr            error
	healthErr          error
	exitAfterStart     bool
	exitAfterStartCode int
	logBlock           bool
	logStarted         chan struct{}
	logStartedOnce     sync.Once
}

type FakeRuntimeOption func(*FakeRuntime)

type FakeOperation struct {
	Name        string
	Image       string
	TaskID      string
	ContainerID ContainerID
	Force       bool
	Labels      map[string]string
	Auth        *RegistryAuth
}

func NewFakeRuntime(opts ...FakeRuntimeOption) *FakeRuntime {
	runtime := &FakeRuntime{
		containers: make(map[ContainerID]ContainerStatus),
		byTask:     make(map[string]ContainerID),
		logs:       make(map[ContainerID][]LogLine),
	}
	for _, opt := range opts {
		opt(runtime)
	}
	return runtime
}

func WithFakeContainerIDs(ids ...ContainerID) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.nextIDs = append([]ContainerID(nil), ids...)
	}
}

func WithFakePullFailure(err error) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.pullErr = err
	}
}

func WithFakeCreateFailure(err error) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.createErr = err
	}
}

func WithFakeStartFailure(err error) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.startErr = err
	}
}

func WithFakeInspectFailure(err error) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.inspectErr = err
	}
}

func WithFakeListFailure(err error) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.listErr = err
	}
}

func WithFakeHealthFailure(err error) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.healthErr = err
	}
}

func WithFakeExitAfterStart(exitCode int) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.exitAfterStart = true
		runtime.exitAfterStartCode = exitCode
	}
}

func WithFakeLogBlock(started chan struct{}) FakeRuntimeOption {
	return func(runtime *FakeRuntime) {
		runtime.logBlock = true
		runtime.logStarted = started
	}
}

func (r *FakeRuntime) PullImage(ctx context.Context, image string, auth *RegistryAuth) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	clonedAuth := cloneRegistryAuth(auth)
	r.operations = append(r.operations, FakeOperation{Name: "pull", Image: image, Auth: clonedAuth})
	r.pullAuths = append(r.pullAuths, clonedAuth)
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("image is required")
	}
	return r.pullErr
}

func (r *FakeRuntime) CreateContainer(ctx context.Context, spec ContainerSpec) (ContainerID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := spec.Validate(); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, FakeOperation{Name: "create", TaskID: spec.TaskID})
	if r.createErr != nil {
		return "", r.createErr
	}
	if id := r.byTask[spec.TaskID]; id != "" {
		return id, nil
	}
	r.createdSpecs = append(r.createdSpecs, cloneContainerSpec(spec))
	id := r.nextContainerIDLocked()
	now := time.Now().UTC()
	labels := fakeManagedLabels(spec)
	r.byTask[spec.TaskID] = id
	r.containers[id] = ContainerStatus{
		ID:        id,
		Name:      spec.Name,
		Image:     spec.Image,
		State:     "created",
		Status:    "created",
		Labels:    labels,
		CreatedAt: now,
	}
	return id, nil
}

func (r *FakeRuntime) StartContainer(ctx context.Context, id ContainerID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, FakeOperation{Name: "start", ContainerID: id})
	if r.startErr != nil {
		return r.startErr
	}
	status, ok := r.containers[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrFakeContainerNotFound, id)
	}
	now := time.Now().UTC()
	status.State = "running"
	status.Status = "running"
	status.Running = true
	status.StartedAt = now
	r.logs[id] = append(r.logs[id], LogLine{Stream: "stdout", Line: "started " + string(id), Timestamp: now})
	if r.exitAfterStart {
		status.State = "exited"
		status.Status = "exited"
		status.Running = false
		status.ExitCode = r.exitAfterStartCode
		if status.ExitCode == 0 {
			status.ExitCode = 1
		}
		status.FinishedAt = now
		r.logs[id] = append(r.logs[id], LogLine{Stream: "stderr", Line: fmt.Sprintf("exited %s with code %d", id, status.ExitCode), Timestamp: now})
	}
	r.containers[id] = status
	return nil
}

func (r *FakeRuntime) StopContainer(ctx context.Context, id ContainerID, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, FakeOperation{Name: "stop", ContainerID: id})
	status, ok := r.containers[id]
	if !ok {
		return nil
	}
	if !status.Running {
		return nil
	}
	status.State = "exited"
	status.Status = "exited"
	status.Running = false
	status.FinishedAt = time.Now().UTC()
	r.containers[id] = status
	return nil
}

func (r *FakeRuntime) RemoveContainer(ctx context.Context, id ContainerID, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, FakeOperation{Name: "remove", ContainerID: id, Force: force})
	status, ok := r.containers[id]
	if !ok {
		return nil
	}
	delete(r.byTask, status.Labels[TaskIDLabel])
	delete(r.containers, id)
	return nil
}

func (r *FakeRuntime) InspectContainer(ctx context.Context, id ContainerID) (ContainerStatus, error) {
	if err := ctx.Err(); err != nil {
		return ContainerStatus{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, FakeOperation{Name: "inspect", ContainerID: id})
	if r.inspectErr != nil {
		return ContainerStatus{}, r.inspectErr
	}
	status, ok := r.containers[id]
	if !ok {
		return ContainerStatus{}, fmt.Errorf("%w: %s", ErrFakeContainerNotFound, id)
	}
	return cloneStatus(status), nil
}

func (r *FakeRuntime) ListManagedContainers(ctx context.Context, labels map[string]string) ([]ContainerStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, FakeOperation{Name: "list", Labels: cloneMap(labels)})
	if r.listErr != nil {
		return nil, r.listErr
	}
	statuses := make([]ContainerStatus, 0, len(r.containers))
	for _, status := range r.containers {
		if !fakeMatchesLabels(status, labels) {
			continue
		}
		statuses = append(statuses, cloneStatus(status))
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].CreatedAt.Equal(statuses[j].CreatedAt) {
			return statuses[i].ID < statuses[j].ID
		}
		return statuses[i].CreatedAt.Before(statuses[j].CreatedAt)
	})
	return statuses, nil
}

func (r *FakeRuntime) StreamLogs(ctx context.Context, id ContainerID, _ LogOptions) (<-chan LogLine, <-chan error) {
	lines := make(chan LogLine)
	errs := make(chan error, 1)
	r.mu.Lock()
	r.operations = append(r.operations, FakeOperation{Name: "logs", ContainerID: id})
	logs := append([]LogLine(nil), r.logs[id]...)
	block := r.logBlock
	started := r.logStarted
	r.mu.Unlock()

	go func() {
		defer close(lines)
		defer close(errs)
		if started != nil {
			r.logStartedOnce.Do(func() { close(started) })
		}
		if block {
			<-ctx.Done()
			return
		}
		for _, line := range logs {
			select {
			case <-ctx.Done():
				return
			case lines <- line:
			}
		}
	}()
	return lines, errs
}

func (r *FakeRuntime) CheckHealth(ctx context.Context, id ContainerID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, FakeOperation{Name: "health", ContainerID: id})
	return r.healthErr
}

func (r *FakeRuntime) AddContainer(status ContainerStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status = cloneStatus(status)
	if status.ID == "" {
		status.ID = r.nextContainerIDLocked()
	}
	if status.CreatedAt.IsZero() {
		status.CreatedAt = time.Now().UTC()
	}
	if status.Labels == nil {
		status.Labels = make(map[string]string)
	}
	r.containers[status.ID] = status
	if taskID := status.Labels[TaskIDLabel]; taskID != "" {
		r.byTask[taskID] = status.ID
	}
}

func (r *FakeRuntime) SetLogs(id ContainerID, logs []LogLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs[id] = append([]LogLine(nil), logs...)
}

func (r *FakeRuntime) Operations() []FakeOperation {
	r.mu.Lock()
	defer r.mu.Unlock()
	operations := make([]FakeOperation, len(r.operations))
	for i, operation := range r.operations {
		operations[i] = operation
		operations[i].Labels = cloneMap(operation.Labels)
		operations[i].Auth = cloneRegistryAuth(operation.Auth)
	}
	return operations
}

func (r *FakeRuntime) PullAuths() []*RegistryAuth {
	r.mu.Lock()
	defer r.mu.Unlock()
	auths := make([]*RegistryAuth, len(r.pullAuths))
	for i, auth := range r.pullAuths {
		auths[i] = cloneRegistryAuth(auth)
	}
	return auths
}

func (r *FakeRuntime) OperationStrings() []string {
	operations := r.Operations()
	values := make([]string, len(operations))
	for i, operation := range operations {
		values[i] = operation.String()
	}
	return values
}

func (r *FakeRuntime) CreatedSpecs() []ContainerSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	specs := make([]ContainerSpec, len(r.createdSpecs))
	for i, spec := range r.createdSpecs {
		specs[i] = cloneContainerSpec(spec)
	}
	return specs
}

func (op FakeOperation) String() string {
	switch op.Name {
	case "pull":
		return "pull:" + op.Image
	case "create":
		return "create:" + op.TaskID
	case "start", "stop", "remove", "inspect", "logs", "health":
		return op.Name + ":" + string(op.ContainerID)
	default:
		return op.Name
	}
}

func (r *FakeRuntime) nextContainerIDLocked() ContainerID {
	if len(r.nextIDs) > 0 {
		id := r.nextIDs[0]
		r.nextIDs = r.nextIDs[1:]
		return id
	}
	r.next++
	return ContainerID("fake-container-" + strconv.Itoa(r.next))
}

func fakeManagedLabels(spec ContainerSpec) map[string]string {
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

func fakeMatchesLabels(status ContainerStatus, labels map[string]string) bool {
	if status.Labels[ManagedLabel] != "true" {
		return false
	}
	for key, value := range labels {
		if status.Labels[key] != value {
			return false
		}
	}
	return true
}

func cloneStatus(status ContainerStatus) ContainerStatus {
	status.Labels = cloneMap(status.Labels)
	return status
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneContainerSpec(spec ContainerSpec) ContainerSpec {
	spec.Env = cloneMap(spec.Env)
	spec.Labels = cloneMap(spec.Labels)
	spec.Ports = append([]PortBinding(nil), spec.Ports...)
	spec.Command = append([]string(nil), spec.Command...)
	return spec
}

func cloneRegistryAuth(auth *RegistryAuth) *RegistryAuth {
	if auth == nil {
		return nil
	}
	cloned := *auth
	return &cloned
}

var _ Runtime = (*FakeRuntime)(nil)
