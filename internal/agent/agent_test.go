package agent

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/config"
	orchdocker "github.com/alekpopovic/orch/internal/docker"
	"github.com/alekpopovic/orch/internal/health"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestReconcileAssignedTaskExecutesRuntimeSteps(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	client := &fakeAgentClient{
		tasks: []api.AgentTask{{
			Task: types.Task{
				ID:            taskID,
				ServiceID:     types.ServiceID("00000000-0000-4000-8000-000000000003"),
				NodeID:        nodeID,
				DesiredStatus: types.TaskRunning,
				ActualStatus:  types.TaskAssigned,
				Image:         "nginx:1.27",
				Version:       1,
			},
		}},
	}
	runtime := &fakeRuntime{createdID: orchdocker.ContainerID("container-1")}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}

	wantRuntimeCalls := []string{"pull:nginx:1.27", "create:" + string(taskID), "start:container-1", "list"}
	if !equalStrings(runtime.calls, wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.calls)
	}
	wantStatuses := []types.TaskStatus{types.TaskPulling, types.TaskCreated, types.TaskRunning}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
}

func TestReconcileRemovesUnassignedManagedContainers(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	staleTaskID := types.TaskID("00000000-0000-4000-8000-000000000004")
	client := &fakeAgentClient{}
	runtime := &fakeRuntime{
		managed: []orchdocker.ContainerStatus{{
			ID: orchdocker.ContainerID("stale-container"),
			Labels: map[string]string{
				orchdocker.TaskIDLabel: string(staleTaskID),
				orchdocker.NodeIDLabel: string(nodeID),
			},
		}},
	}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}

	wantRuntimeCalls := []string{"list", "stop:stale-container", "remove:stale-container"}
	if !equalStrings(runtime.calls, wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.calls)
	}
	wantStatuses := []types.TaskStatus{types.TaskRemoved}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
}

func TestReconcileAssignedTaskReportsHealthAfterThresholds(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	task := api.AgentTask{
		Task: types.Task{
			ID:            taskID,
			ServiceID:     types.ServiceID("00000000-0000-4000-8000-000000000003"),
			NodeID:        nodeID,
			ContainerID:   "container-1",
			DesiredStatus: types.TaskRunning,
			ActualStatus:  types.TaskRunning,
			Image:         "nginx:1.27",
			Version:       1,
		},
		Healthcheck: &types.Healthcheck{
			Type:               types.HealthcheckHTTP,
			Path:               "/health",
			Port:               8080,
			HealthyThreshold:   2,
			UnhealthyThreshold: 2,
		},
	}
	client := &fakeAgentClient{tasks: []api.AgentTask{task}}
	runtime := &fakeRuntime{inspect: map[orchdocker.ContainerID]orchdocker.ContainerStatus{
		"container-1": {ID: "container-1", Running: true},
	}}
	checker := &fakeHealthChecker{results: []bool{true, true, false, false}}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRuntime(runtime).
		WithHealthChecker(checker)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("third reconcile: %v", err)
	}
	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("fourth reconcile: %v", err)
	}

	wantStatuses := []types.TaskStatus{types.TaskHealthy, types.TaskUnhealthy}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
}

type fakeAgentClient struct {
	tasks    []api.AgentTask
	statuses []types.TaskStatus
}

func (c *fakeAgentClient) Register(context.Context, api.AgentRegisterRequest) (api.AgentResponse, error) {
	return api.AgentResponse{}, nil
}

func (c *fakeAgentClient) Heartbeat(context.Context, api.AgentHeartbeatRequest) (api.AgentResponse, error) {
	return api.AgentResponse{}, nil
}

func (c *fakeAgentClient) ListAssignedTasks(context.Context, types.NodeID) ([]api.AgentTask, error) {
	return c.tasks, nil
}

func (c *fakeAgentClient) ReportTaskStatus(_ context.Context, taskID types.TaskID, req api.AgentTaskStatusRequest) (types.Task, error) {
	c.statuses = append(c.statuses, req.Status)
	return types.Task{
		ID:            taskID,
		NodeID:        req.NodeID,
		ActualStatus:  req.Status,
		ContainerID:   req.ContainerID,
		FailureReason: req.FailureReason,
	}, nil
}

type fakeRuntime struct {
	calls     []string
	createdID orchdocker.ContainerID
	managed   []orchdocker.ContainerStatus
	inspect   map[orchdocker.ContainerID]orchdocker.ContainerStatus
}

func (r *fakeRuntime) PullImage(_ context.Context, image string, _ *orchdocker.RegistryAuth) error {
	r.calls = append(r.calls, "pull:"+image)
	return nil
}

func (r *fakeRuntime) CreateContainer(_ context.Context, spec orchdocker.ContainerSpec) (orchdocker.ContainerID, error) {
	r.calls = append(r.calls, "create:"+spec.TaskID)
	return r.createdID, nil
}

func (r *fakeRuntime) StartContainer(_ context.Context, id orchdocker.ContainerID) error {
	r.calls = append(r.calls, "start:"+string(id))
	return nil
}

func (r *fakeRuntime) StopContainer(_ context.Context, id orchdocker.ContainerID, _ time.Duration) error {
	r.calls = append(r.calls, "stop:"+string(id))
	return nil
}

func (r *fakeRuntime) RemoveContainer(_ context.Context, id orchdocker.ContainerID, _ bool) error {
	r.calls = append(r.calls, "remove:"+string(id))
	return nil
}

func (r *fakeRuntime) InspectContainer(_ context.Context, id orchdocker.ContainerID) (orchdocker.ContainerStatus, error) {
	r.calls = append(r.calls, "inspect:"+string(id))
	if r.inspect == nil {
		return orchdocker.ContainerStatus{}, errFakeNotFound{}
	}
	status, ok := r.inspect[id]
	if !ok {
		return orchdocker.ContainerStatus{}, errFakeNotFound{}
	}
	return status, nil
}

func (r *fakeRuntime) ListManagedContainers(_ context.Context, _ map[string]string) ([]orchdocker.ContainerStatus, error) {
	r.calls = append(r.calls, "list")
	return r.managed, nil
}

func (r *fakeRuntime) StreamLogs(context.Context, orchdocker.ContainerID, orchdocker.LogOptions) (<-chan orchdocker.LogLine, <-chan error) {
	lines := make(chan orchdocker.LogLine)
	errs := make(chan error)
	close(lines)
	close(errs)
	return lines, errs
}

type errFakeNotFound struct{}

func (errFakeNotFound) Error() string {
	return "not found"
}

type fakeHealthChecker struct {
	results []bool
	calls   int
}

func (c *fakeHealthChecker) Check(context.Context, health.Check) (health.Result, error) {
	result := false
	if c.calls < len(c.results) {
		result = c.results[c.calls]
	}
	c.calls++
	return health.Result{Healthy: result, CheckedAt: time.Now().UTC(), Message: "fake health result"}, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalStatuses(left, right []types.TaskStatus) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
