package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/config"
	orchdocker "github.com/alekpopovic/orch/internal/docker"
	"github.com/alekpopovic/orch/internal/health"
	"github.com/alekpopovic/orch/internal/metrics"
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
	runtime := orchdocker.NewFakeRuntime(orchdocker.WithFakeContainerIDs("container-1"))
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}

	wantRuntimeCalls := []string{"list", "pull:nginx:1.27", "create:" + string(taskID), "start:container-1", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	wantStatuses := []types.TaskStatus{types.TaskPulling, types.TaskCreated, types.TaskRunning}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
}

func TestReconcileAssignedTaskRecreatesManuallyDeletedContainer(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	client := &fakeAgentClient{tasks: []api.AgentTask{{
		Task: types.Task{
			ID:            taskID,
			ServiceID:     types.ServiceID("00000000-0000-4000-8000-000000000003"),
			NodeID:        nodeID,
			ContainerID:   "missing-container",
			DesiredStatus: types.TaskRunning,
			ActualStatus:  types.TaskRunning,
			Image:         "nginx:1.27",
			Version:       1,
		},
	}}}
	runtime := orchdocker.NewFakeRuntime(orchdocker.WithFakeContainerIDs("replacement-container"))
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}
	wantRuntimeCalls := []string{"inspect:missing-container", "list", "pull:nginx:1.27", "create:" + string(taskID), "start:replacement-container", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	wantStatuses := []types.TaskStatus{types.TaskPulling, types.TaskCreated, types.TaskRunning}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
}

func TestReconcileAssignedTaskReReportsRunningContainerAfterAgentRestart(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	client := &fakeAgentClient{tasks: []api.AgentTask{{
		Task: types.Task{
			ID:            taskID,
			ServiceID:     types.ServiceID("00000000-0000-4000-8000-000000000003"),
			NodeID:        nodeID,
			DesiredStatus: types.TaskRunning,
			ActualStatus:  types.TaskAssigned,
			Image:         "nginx:1.27",
			Version:       1,
		},
	}}}
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(orchdocker.ContainerStatus{
		ID:      "container-1",
		Running: true,
		Labels: map[string]string{
			orchdocker.ManagedLabel: "true",
			orchdocker.TaskIDLabel:  string(taskID),
			orchdocker.NodeIDLabel:  string(nodeID),
		},
	})
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}
	wantRuntimeCalls := []string{"list", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	wantStatuses := []types.TaskStatus{types.TaskRunning}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
	if client.containerIDs[0] != "container-1" {
		t.Fatalf("expected recovered container id, got %q", client.containerIDs[0])
	}
}

func TestReconcileAssignedTaskDoesNotRecreateWhenDockerUnavailable(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	client := &fakeAgentClient{tasks: []api.AgentTask{{
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
	}}}
	runtime := orchdocker.NewFakeRuntime(
		orchdocker.WithFakeInspectFailure(errDockerUnavailable),
		orchdocker.WithFakeListFailure(errDockerUnavailable),
	)
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != errDockerUnavailable {
		t.Fatalf("expected docker unavailable error, got %v", err)
	}
	wantRuntimeCalls := []string{"inspect:container-1", "list", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	if len(client.statuses) != 0 {
		t.Fatalf("expected no status reports while Docker availability is unknown, got %#v", client.statuses)
	}
}

func TestReconcileAssignedTaskReportsExitedContainerFailed(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	client := &fakeAgentClient{tasks: []api.AgentTask{{
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
	}}}
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(orchdocker.ContainerStatus{ID: "container-1", Running: false, ExitCode: 2})
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err == nil {
		t.Fatalf("expected exited container error")
	}
	wantStatuses := []types.TaskStatus{types.TaskFailed}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
	if client.failureReasons[0] != "container exited with code 2" {
		t.Fatalf("unexpected failure reason %q", client.failureReasons[0])
	}
}

func TestReconcileAssignedTaskReportsPullAndCreateFailures(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	tests := []struct {
		name         string
		runtime      *orchdocker.FakeRuntime
		wantCalls    []string
		wantReason   string
		wantStatuses []types.TaskStatus
	}{
		{
			name:         "pull failure",
			runtime:      orchdocker.NewFakeRuntime(orchdocker.WithFakePullFailure(errors.New("pull failed"))),
			wantCalls:    []string{"list", "pull:nginx:1.27", "list"},
			wantReason:   "pull failed",
			wantStatuses: []types.TaskStatus{types.TaskPulling, types.TaskFailed},
		},
		{
			name:         "create failure",
			runtime:      orchdocker.NewFakeRuntime(orchdocker.WithFakeCreateFailure(errors.New("port is already allocated"))),
			wantCalls:    []string{"list", "pull:nginx:1.27", "create:" + string(taskID), "list"},
			wantReason:   "port is already allocated",
			wantStatuses: []types.TaskStatus{types.TaskPulling, types.TaskFailed},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeAgentClient{tasks: []api.AgentTask{{
				Task: types.Task{
					ID:            taskID,
					ServiceID:     types.ServiceID("00000000-0000-4000-8000-000000000003"),
					NodeID:        nodeID,
					DesiredStatus: types.TaskRunning,
					ActualStatus:  types.TaskAssigned,
					Image:         "nginx:1.27",
					Version:       1,
				},
			}}}
			runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(tt.runtime)

			if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err == nil {
				t.Fatalf("expected task execution error")
			}
			if !equalStrings(tt.runtime.OperationStrings(), tt.wantCalls) {
				t.Fatalf("expected runtime calls %#v, got %#v", tt.wantCalls, tt.runtime.OperationStrings())
			}
			if !equalStatuses(client.statuses, tt.wantStatuses) {
				t.Fatalf("expected statuses %#v, got %#v", tt.wantStatuses, client.statuses)
			}
			if client.failureReasons[len(client.failureReasons)-1] != tt.wantReason {
				t.Fatalf("expected failure reason %q, got %#v", tt.wantReason, client.failureReasons)
			}
		})
	}
}

func TestReconcileRemovesUnassignedManagedContainers(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	staleTaskID := types.TaskID("00000000-0000-4000-8000-000000000004")
	client := &fakeAgentClient{}
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(orchdocker.ContainerStatus{
		ID: orchdocker.ContainerID("stale-container"),
		Labels: map[string]string{
			orchdocker.ManagedLabel: "true",
			orchdocker.TaskIDLabel:  string(staleTaskID),
			orchdocker.NodeIDLabel:  string(nodeID),
		},
	})
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}

	wantRuntimeCalls := []string{"list", "stop:stale-container", "remove:stale-container"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	wantStatuses := []types.TaskStatus{types.TaskRemoved}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
}

func TestReconcileRemovesAssignedStoppedTask(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	client := &fakeAgentClient{
		tasks: []api.AgentTask{{
			Task: types.Task{
				ID:            taskID,
				ServiceID:     types.ServiceID("00000000-0000-4000-8000-000000000003"),
				NodeID:        nodeID,
				ContainerID:   "container-1",
				DesiredStatus: types.TaskStopped,
				ActualStatus:  types.TaskRunning,
				Image:         "nginx:1.27",
				Version:       1,
			},
		}},
	}
	runtime := orchdocker.NewFakeRuntime()
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}

	wantRuntimeCalls := []string{"stop:container-1", "remove:container-1", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
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
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(orchdocker.ContainerStatus{ID: "container-1", Running: true})
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

func TestLogHandlerStopsOnCancellation(t *testing.T) {
	logStarted := make(chan struct{})
	runtime := orchdocker.NewFakeRuntime(orchdocker.WithFakeLogBlock(logStarted))
	runtime.AddContainer(orchdocker.ContainerStatus{
		ID: "container-1",
		Labels: map[string]string{
			orchdocker.ManagedLabel: "true",
			orchdocker.TaskIDLabel:  "task-1",
		},
	})
	handler := NewLogHandler(runtime, "secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/agent/logs?task_id=task-1&follow=true", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	<-logStarted
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("log handler did not stop after cancellation")
	}
}

func TestLogHandlerExposesMetrics(t *testing.T) {
	agentMetrics := metrics.NewAgent()
	handler := NewLogHandler(orchdocker.NewFakeRuntime(), "secret", slog.New(slog.NewTextHandler(io.Discard, nil)), WithMetricsHandler(agentMetrics.Handler()))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "heartbeat_success_total") {
		t.Fatalf("expected agent metrics, got %s", rec.Body.String())
	}
}

type fakeAgentClient struct {
	tasks          []api.AgentTask
	statuses       []types.TaskStatus
	failureReasons []string
	containerIDs   []string
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
	c.failureReasons = append(c.failureReasons, req.FailureReason)
	c.containerIDs = append(c.containerIDs, req.ContainerID)
	return types.Task{
		ID:            taskID,
		NodeID:        req.NodeID,
		ActualStatus:  req.Status,
		ContainerID:   req.ContainerID,
		FailureReason: req.FailureReason,
	}, nil
}

var errDockerUnavailable = errors.New("docker unavailable")

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
