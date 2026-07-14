package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/config"
	"github.com/alekpopovic/orch/internal/controlplane"
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
			Ports:         []types.Port{{Protocol: types.PortTCP, ContainerPort: 8080, PublishedPort: 18080}},
			Env:           map[string]string{"DATABASE_URL": "postgres://secret"},
			ImagePullAuth: &controlplane.RegistryAuth{Username: "robot", Password: "token", ServerAddress: "ghcr.io"},
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
	pullAuths := runtime.PullAuths()
	if len(pullAuths) != 1 || pullAuths[0].Username != "robot" || pullAuths[0].Password != "token" || pullAuths[0].ServerAddress != "ghcr.io" {
		t.Fatalf("expected Docker pull auth, got %#v", pullAuths)
	}
	specs := runtime.CreatedSpecs()
	if len(specs) != 1 || specs[0].Env["DATABASE_URL"] != "postgres://secret" {
		t.Fatalf("expected secret env in container spec, got %#v", specs)
	}
	wantStatuses := []types.TaskStatus{types.TaskPulling, types.TaskCreated, types.TaskRunning}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
	created := runtime.CreatedSpecs()
	if len(created) != 1 {
		t.Fatalf("expected one created container spec, got %#v", created)
	}
	if len(created[0].Ports) != 1 || created[0].Ports[0].ContainerPort != 8080 || created[0].Ports[0].HostPort != 18080 {
		t.Fatalf("expected assigned port binding, got %#v", created[0].Ports)
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
	drift := &fakeDriftRecorder{}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRuntime(runtime).
		WithDriftRecorder(drift)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}
	if !drift.has(DriftManagedContainerMissing) {
		t.Fatalf("expected missing container drift event, got %#v", drift.events)
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
	runtime.AddContainer(managedTaskContainer("container-1", nodeID, taskID, types.ServiceID("00000000-0000-4000-8000-000000000003"), "nginx:1.27", 1, true))
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
	exited := managedTaskContainer("container-1", nodeID, taskID, types.ServiceID("00000000-0000-4000-8000-000000000003"), "nginx:1.27", 1, false)
	exited.ExitCode = 2
	runtime.AddContainer(exited)
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

func TestReconcileAssignedTaskRestartsManuallyStoppedContainer(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	serviceID := types.ServiceID("00000000-0000-4000-8000-000000000003")
	client := &fakeAgentClient{tasks: []api.AgentTask{{
		Task: types.Task{
			ID:            taskID,
			ServiceID:     serviceID,
			NodeID:        nodeID,
			ContainerID:   "container-1",
			DesiredStatus: types.TaskRunning,
			ActualStatus:  types.TaskRunning,
			Image:         "nginx:1.27",
			Version:       1,
		},
	}}}
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(managedTaskContainer("container-1", nodeID, taskID, serviceID, "nginx:1.27", 1, false))
	drift := &fakeDriftRecorder{}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRuntime(runtime).
		WithDriftRecorder(drift)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile stopped container: %v", err)
	}

	wantRuntimeCalls := []string{"inspect:container-1", "start:container-1", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	if !drift.has(DriftManagedContainerStateMismatch) {
		t.Fatalf("expected state mismatch drift event, got %#v", drift.events)
	}
	wantStatuses := []types.TaskStatus{types.TaskRunning}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
}

func TestReconcileAssignedTaskReplacesMismatchedManagedContainer(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	serviceID := types.ServiceID("00000000-0000-4000-8000-000000000003")
	client := &fakeAgentClient{tasks: []api.AgentTask{{
		Task: types.Task{
			ID:            taskID,
			ServiceID:     serviceID,
			NodeID:        nodeID,
			ContainerID:   "container-1",
			DesiredStatus: types.TaskRunning,
			ActualStatus:  types.TaskRunning,
			Image:         "nginx:1.27",
			Version:       2,
		},
	}}}
	runtime := orchdocker.NewFakeRuntime(orchdocker.WithFakeContainerIDs("replacement-container"))
	runtime.AddContainer(managedTaskContainer("container-1", nodeID, taskID, serviceID, "nginx:1.26", 1, true))
	drift := &fakeDriftRecorder{}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRuntime(runtime).
		WithDriftRecorder(drift)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile mismatched container: %v", err)
	}

	wantRuntimeCalls := []string{"inspect:container-1", "stop:container-1", "remove:container-1", "list", "pull:nginx:1.27", "create:" + string(taskID), "start:replacement-container", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	if !drift.has(DriftManagedContainerStateMismatch) {
		t.Fatalf("expected state mismatch drift event, got %#v", drift.events)
	}
	wantStatuses := []types.TaskStatus{types.TaskPulling, types.TaskCreated, types.TaskRunning}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
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
			wantReason:   "port allocation failed: port is already allocated",
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
	drift := &fakeDriftRecorder{}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRuntime(runtime).
		WithDriftRecorder(drift)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}
	if !drift.has(DriftUnexpectedManagedContainer) {
		t.Fatalf("expected unexpected container drift event, got %#v", drift.events)
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

func TestReconcileIgnoresNonManagedContainers(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	client := &fakeAgentClient{}
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(orchdocker.ContainerStatus{
		ID: "foreign-container",
		Labels: map[string]string{
			orchdocker.NodeIDLabel: "00000000-0000-4000-8000-000000000001",
			orchdocker.TaskIDLabel: "foreign-task",
		},
	})
	drift := &fakeDriftRecorder{}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRuntime(runtime).
		WithDriftRecorder(drift)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}

	wantRuntimeCalls := []string{"list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	if len(client.statuses) != 0 {
		t.Fatalf("expected no status reports, got %#v", client.statuses)
	}
	if len(drift.events) != 0 {
		t.Fatalf("expected no drift events for non-managed container, got %#v", drift.events)
	}
}

func TestReturnedNodeRemovesNodeLostUnassignedContainer(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	lostTaskID := types.TaskID("00000000-0000-4000-8000-000000000004")
	client := &fakeAgentClient{}
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(orchdocker.ContainerStatus{
		ID: "lost-container",
		Labels: map[string]string{
			orchdocker.ManagedLabel: "true",
			orchdocker.TaskIDLabel:  string(lostTaskID),
			orchdocker.NodeIDLabel:  string(nodeID),
		},
	})
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile returned node: %v", err)
	}

	wantRuntimeCalls := []string{"list", "stop:lost-container", "remove:lost-container"}
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
	runtime.AddContainer(managedTaskContainer("container-1", nodeID, taskID, types.ServiceID("00000000-0000-4000-8000-000000000003"), "nginx:1.27", 1, true))
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRuntime(runtime)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}

	wantRuntimeCalls := []string{"inspect:container-1", "stop:container-1", "remove:container-1", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	wantStatuses := []types.TaskStatus{types.TaskRemoved}
	if !equalStatuses(client.statuses, wantStatuses) {
		t.Fatalf("expected statuses %#v, got %#v", wantStatuses, client.statuses)
	}
}

func TestReconcileDoesNotRemoveNonManagedAssignedContainerID(t *testing.T) {
	nodeID := types.NodeID("00000000-0000-4000-8000-000000000001")
	taskID := types.TaskID("00000000-0000-4000-8000-000000000002")
	client := &fakeAgentClient{
		tasks: []api.AgentTask{{
			Task: types.Task{
				ID:            taskID,
				ServiceID:     types.ServiceID("00000000-0000-4000-8000-000000000003"),
				NodeID:        nodeID,
				ContainerID:   "foreign-container",
				DesiredStatus: types.TaskStopped,
				ActualStatus:  types.TaskRunning,
				Image:         "nginx:1.27",
				Version:       1,
			},
		}},
	}
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(orchdocker.ContainerStatus{ID: "foreign-container", Running: true})
	drift := &fakeDriftRecorder{}
	runner := NewRunner(config.AgentConfig{}, client, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRuntime(runtime).
		WithDriftRecorder(drift)

	if err := runner.reconcileAssignedTasks(context.Background(), nodeID); err != nil {
		t.Fatalf("reconcile assigned tasks: %v", err)
	}

	wantRuntimeCalls := []string{"inspect:foreign-container", "list", "list"}
	if !equalStrings(runtime.OperationStrings(), wantRuntimeCalls) {
		t.Fatalf("expected runtime calls %#v, got %#v", wantRuntimeCalls, runtime.OperationStrings())
	}
	if !drift.has(DriftManagedContainerStateMismatch) {
		t.Fatalf("expected state mismatch drift event, got %#v", drift.events)
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
		Ports: []types.Port{{
			ContainerPort: 8080,
			PublishedPort: 18080,
			Protocol:      types.PortTCP,
		}},
	}
	client := &fakeAgentClient{tasks: []api.AgentTask{task}}
	runtime := orchdocker.NewFakeRuntime()
	runtime.AddContainer(managedTaskContainer("container-1", nodeID, taskID, types.ServiceID("00000000-0000-4000-8000-000000000003"), "nginx:1.27", 1, true))
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

func TestHealthProbeRequiresAssignedPort(t *testing.T) {
	check := types.Healthcheck{
		Type: types.HealthcheckHTTP,
		Path: "/health",
		Port: 2375,
	}
	ports := []types.Port{{
		ContainerPort: 8080,
		PublishedPort: 18080,
		Protocol:      types.PortTCP,
	}}

	if _, ok := healthProbe(check, ports); ok {
		t.Fatalf("expected health probe for unassigned host port to be rejected")
	}

	check.Port = 9090
	if _, ok := healthProbe(check, []types.Port{{ContainerPort: 9090, Protocol: types.PortTCP}}); ok {
		t.Fatalf("expected health probe without a published port to be rejected")
	}

	check.Port = 5353
	if _, ok := healthProbe(check, []types.Port{{ContainerPort: 5353, PublishedPort: 15353, Protocol: types.PortUDP}}); ok {
		t.Fatalf("expected health probe for UDP-only port to be rejected")
	}

	check.Port = 8080
	probe, ok := healthProbe(check, ports)
	if !ok {
		t.Fatalf("expected health probe for assigned container port")
	}
	if probe.Target != "http://127.0.0.1:18080/health" {
		t.Fatalf("expected probe to use published port, got %q", probe.Target)
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

type fakeDriftRecorder struct {
	events []DriftEvent
}

func (r *fakeDriftRecorder) RecordDrift(event DriftEvent) {
	r.events = append(r.events, event)
}

func (r *fakeDriftRecorder) has(eventType DriftEventType) bool {
	for _, event := range r.events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func managedTaskContainer(id orchdocker.ContainerID, nodeID types.NodeID, taskID types.TaskID, serviceID types.ServiceID, image string, version int64, running bool) orchdocker.ContainerStatus {
	status := orchdocker.ContainerStatus{
		ID:      id,
		Image:   image,
		Running: running,
		Labels: map[string]string{
			orchdocker.ManagedLabel:   "true",
			orchdocker.TaskIDLabel:    string(taskID),
			orchdocker.NodeIDLabel:    string(nodeID),
			orchdocker.ServiceIDLabel: string(serviceID),
			orchdocker.VersionLabel:   strconv.FormatInt(version, 10),
		},
	}
	if running {
		status.State = "running"
		status.Status = "running"
	} else {
		status.State = "exited"
		status.Status = "exited"
	}
	return status
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
