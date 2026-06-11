package controlplane

import (
	"context"
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestAgentHeartbeatStateTransitions(t *testing.T) {
	service := NewMemoryService()
	ctx := context.Background()

	registered, err := service.RegisterNode(ctx, NodeRegistration{
		Name:             "node-a",
		AdvertiseAddress: "10.0.0.10",
		Labels:           map[string]string{"role": "app"},
		Capacity:         types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
		Allocatable:      types.Resources{CPU: 3500, Memory: 7 * 1024 * 1024 * 1024},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}
	if registered.Node.Status != types.NodeReady {
		t.Fatalf("expected ready after register, got %q", registered.Node.Status)
	}

	drained, err := service.DrainNode(ctx, registered.Node.ID)
	if err != nil {
		t.Fatalf("drain node: %v", err)
	}
	if drained.Status != types.NodeDraining {
		t.Fatalf("expected draining after drain, got %q", drained.Status)
	}

	heartbeat, err := service.HeartbeatNode(ctx, NodeHeartbeat{
		NodeID:      registered.Node.ID,
		Capacity:    registered.Node.Capacity,
		Allocatable: registered.Node.Allocatable,
	})
	if err != nil {
		t.Fatalf("heartbeat node: %v", err)
	}
	if heartbeat.Node.Status != types.NodeDraining {
		t.Fatalf("heartbeat should preserve server draining status, got %q", heartbeat.Node.Status)
	}
	if len(heartbeat.Directives) != 1 || heartbeat.Directives[0].Type != "drain" {
		t.Fatalf("expected drain directive, got %#v", heartbeat.Directives)
	}

	offline, err := service.HeartbeatNode(ctx, NodeHeartbeat{
		NodeID:   registered.Node.ID,
		Shutdown: true,
	})
	if err != nil {
		t.Fatalf("shutdown heartbeat: %v", err)
	}
	if offline.Node.Status != types.NodeOffline {
		t.Fatalf("expected offline after shutdown heartbeat, got %q", offline.Node.Status)
	}
}

func TestAgentTaskStatusTransitions(t *testing.T) {
	service := NewMemoryService()
	ctx := context.Background()

	registered, err := service.RegisterNode(ctx, NodeRegistration{
		Name:             "node-a",
		AdvertiseAddress: "10.0.0.10",
		Capacity:         types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
		Allocatable:      types.Resources{CPU: 3500, Memory: 7 * 1024 * 1024 * 1024},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}
	created, err := service.CreateService(ctx, types.ServiceSpec{
		Name:                 "api",
		Image:                "ghcr.io/example/api:1.0.0",
		Replicas:             1,
		ResourceRequirements: types.ResourceRequirements{},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	tasks, err := service.ListAssignedTasks(ctx, registered.Node.ID)
	if err != nil {
		t.Fatalf("list assigned tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one assigned task, got %d", len(tasks))
	}
	if tasks[0].ServiceID != created.ID {
		t.Fatalf("expected task for service %q, got %q", created.ID, tasks[0].ServiceID)
	}

	tests := []struct {
		name          string
		status        types.TaskStatus
		containerID   string
		failureReason string
		wantFinished  bool
	}{
		{name: "pulling", status: types.TaskPulling},
		{name: "created", status: types.TaskCreated, containerID: "container-1"},
		{name: "running", status: types.TaskRunning, containerID: "container-1"},
		{name: "unhealthy", status: types.TaskUnhealthy, containerID: "container-1"},
		{name: "failed", status: types.TaskFailed, containerID: "container-1", failureReason: "exit 1", wantFinished: true},
		{name: "stopped", status: types.TaskStopped, containerID: "container-1", wantFinished: true},
		{name: "removed", status: types.TaskRemoved, containerID: "container-1", wantFinished: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := service.ReportTaskStatus(ctx, TaskStatusReport{
				TaskID:        tasks[0].ID,
				NodeID:        registered.Node.ID,
				Status:        tt.status,
				ContainerID:   tt.containerID,
				FailureReason: tt.failureReason,
			})
			if err != nil {
				t.Fatalf("report task status: %v", err)
			}
			if task.ActualStatus != tt.status {
				t.Fatalf("expected status %q, got %q", tt.status, task.ActualStatus)
			}
			if task.ContainerID != tt.containerID {
				t.Fatalf("expected container id %q, got %q", tt.containerID, task.ContainerID)
			}
			if task.FailureReason != tt.failureReason {
				t.Fatalf("expected failure reason %q, got %q", tt.failureReason, task.FailureReason)
			}
			if tt.wantFinished && task.FinishedAt.IsZero() {
				t.Fatalf("expected finished timestamp")
			}
			if tt.status == types.TaskRunning && task.StartedAt.IsZero() {
				t.Fatalf("expected started timestamp")
			}
		})
	}
}

func TestTaskAssignmentReconcilesWhenNodeRegistersAfterService(t *testing.T) {
	service := NewMemoryService()
	ctx := context.Background()

	created, err := service.CreateService(ctx, types.ServiceSpec{
		Name:                 "api",
		Image:                "ghcr.io/example/api:1.0.0",
		Replicas:             1,
		ResourceRequirements: types.ResourceRequirements{},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	registered, err := service.RegisterNode(ctx, NodeRegistration{
		Name:             "node-a",
		AdvertiseAddress: "10.0.0.10",
		Capacity:         types.Resources{CPU: 4000, Memory: 1024},
		Allocatable:      types.Resources{CPU: 3000, Memory: 512},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	tasks, err := service.ListAssignedTasks(ctx, registered.Node.ID)
	if err != nil {
		t.Fatalf("list assigned tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one assigned task, got %d", len(tasks))
	}
	if tasks[0].ServiceID != created.ID {
		t.Fatalf("expected task for service %q, got %q", created.ID, tasks[0].ServiceID)
	}
}

func TestRolloutCreatesReplacementTasks(t *testing.T) {
	service := NewMemoryService()
	ctx := context.Background()

	registered, err := service.RegisterNode(ctx, NodeRegistration{
		Name:             "node-a",
		AdvertiseAddress: "10.0.0.10",
		Capacity:         types.Resources{CPU: 4000, Memory: 1024},
		Allocatable:      types.Resources{CPU: 3000, Memory: 512},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}
	created, err := service.CreateService(ctx, types.ServiceSpec{
		Name:                 "api",
		Image:                "ghcr.io/example/api:1.0.0",
		Replicas:             1,
		ResourceRequirements: types.ResourceRequirements{},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	before, err := service.ListAssignedTasks(ctx, registered.Node.ID)
	if err != nil {
		t.Fatalf("list tasks before rollout: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("expected one task before rollout, got %d", len(before))
	}

	if _, err := service.RolloutService(ctx, created.ID, "ghcr.io/example/api:2.0.0"); err != nil {
		t.Fatalf("rollout service: %v", err)
	}
	after, err := service.ListAssignedTasks(ctx, registered.Node.ID)
	if err != nil {
		t.Fatalf("list tasks after rollout: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected one active task after rollout, got %d", len(after))
	}
	if after[0].ID == before[0].ID {
		t.Fatalf("expected replacement task id, got same task %q", after[0].ID)
	}
	if after[0].Image != "ghcr.io/example/api:2.0.0" {
		t.Fatalf("expected rollout image, got %q", after[0].Image)
	}

	allTasks, err := service.ListTasks(ctx, TaskFilter{ServiceID: created.ID})
	if err != nil {
		t.Fatalf("list all service tasks: %v", err)
	}
	if len(allTasks) != 2 {
		t.Fatalf("expected old and replacement tasks, got %d", len(allTasks))
	}
}
