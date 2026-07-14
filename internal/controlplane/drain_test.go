package controlplane

import (
	"context"
	"testing"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestDrainCreatesReplacementAndStopsOldTaskAfterReplacementRuns(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	nodeA := registerDrainTestNode(t, ctx, service, "node-a")
	created := createDrainTestService(t, ctx, service, 1)
	nodeB := registerDrainTestNode(t, ctx, service, "node-b")
	oldTask := singleTaskOnNode(t, ctx, service, created.ID, nodeA.Node.ID)

	if _, err := service.DrainNode(ctx, nodeA.Node.ID); err != nil {
		t.Fatalf("drain node: %v", err)
	}
	status, err := service.GetNodeDrainStatus(ctx, nodeA.Node.ID)
	if err != nil {
		t.Fatalf("get drain status: %v", err)
	}
	if status.Phase != DrainPending || status.ReplacementTasks != 1 || status.RemainingTasks != 1 {
		t.Fatalf("unexpected pending drain status %#v", status)
	}

	replacement := singleTaskOnNode(t, ctx, service, created.ID, nodeB.Node.ID)
	if replacement.ID == oldTask.ID {
		t.Fatalf("expected replacement task, got old task")
	}
	if _, err := service.ReportTaskStatus(ctx, TaskStatusReport{
		TaskID:      replacement.ID,
		NodeID:      nodeB.Node.ID,
		Status:      types.TaskRunning,
		ContainerID: "replacement-container",
	}); err != nil {
		t.Fatalf("report replacement running: %v", err)
	}

	old, err := service.GetTask(ctx, oldTask.ID)
	if err != nil {
		t.Fatalf("get old task: %v", err)
	}
	if old.DesiredStatus != types.TaskStopped {
		t.Fatalf("expected old task to be stopped after replacement runs, got %#v", old)
	}
	status, err = service.GetNodeDrainStatus(ctx, nodeA.Node.ID)
	if err != nil {
		t.Fatalf("get final drain status: %v", err)
	}
	if status.Phase != DrainComplete {
		t.Fatalf("expected drain complete, got %#v", status)
	}
}

func TestDrainWaitsWithInsufficientCapacity(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	node := registerDrainTestNode(t, ctx, service, "node-a")
	createDrainTestService(t, ctx, service, 1)

	if _, err := service.DrainNode(ctx, node.Node.ID); err != nil {
		t.Fatalf("drain node: %v", err)
	}
	status, err := service.GetNodeDrainStatus(ctx, node.Node.ID)
	if err != nil {
		t.Fatalf("get drain status: %v", err)
	}
	if status.Phase != DrainPending || !status.InsufficientCapacity {
		t.Fatalf("expected insufficient capacity pending drain, got %#v", status)
	}
	items, err := service.ListEvents(ctx, events.Filter{NodeID: node.Node.ID, Type: events.TypeNodeDrainPending})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected warning drain event")
	}
}

func TestUncordonBeforeDrainCompletion(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	nodeA := registerDrainTestNode(t, ctx, service, "node-a")
	created := createDrainTestService(t, ctx, service, 1)
	registerDrainTestNode(t, ctx, service, "node-b")
	oldTask := singleTaskOnNode(t, ctx, service, created.ID, nodeA.Node.ID)

	if _, err := service.DrainNode(ctx, nodeA.Node.ID); err != nil {
		t.Fatalf("drain node: %v", err)
	}
	uncordoned, err := service.UncordonNode(ctx, nodeA.Node.ID)
	if err != nil {
		t.Fatalf("uncordon node: %v", err)
	}
	if uncordoned.Status != types.NodeReady {
		t.Fatalf("expected node ready after uncordon, got %q", uncordoned.Status)
	}
	old, err := service.GetTask(ctx, oldTask.ID)
	if err != nil {
		t.Fatalf("get old task: %v", err)
	}
	if old.DesiredStatus != types.TaskRunning {
		t.Fatalf("expected old task to keep running before replacement is ready, got %#v", old)
	}
	status, err := service.GetNodeDrainStatus(ctx, nodeA.Node.ID)
	if err != nil {
		t.Fatalf("get drain status: %v", err)
	}
	if status.Phase != DrainNotDraining {
		t.Fatalf("expected not draining after uncordon, got %#v", status)
	}
}

func TestNodeOfflineDuringDrainStatus(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	nodeA := registerDrainTestNode(t, ctx, service, "node-a")
	createDrainTestService(t, ctx, service, 1)
	registerDrainTestNode(t, ctx, service, "node-b")

	if _, err := service.DrainNode(ctx, nodeA.Node.ID); err != nil {
		t.Fatalf("drain node: %v", err)
	}
	if _, err := service.HeartbeatNode(ctx, NodeHeartbeat{NodeID: nodeA.Node.ID, Shutdown: true}); err != nil {
		t.Fatalf("shutdown heartbeat: %v", err)
	}
	status, err := service.GetNodeDrainStatus(ctx, nodeA.Node.ID)
	if err != nil {
		t.Fatalf("get drain status: %v", err)
	}
	if status.Phase != DrainOffline {
		t.Fatalf("expected offline drain status, got %#v", status)
	}
}

func registerDrainTestNode(t *testing.T, ctx context.Context, service *MemoryService, name string) NodeCommand {
	t.Helper()
	registered, err := service.RegisterNode(ctx, NodeRegistration{
		Name:             name,
		AdvertiseAddress: "10.0.0.10",
		Capacity:         types.Resources{CPU: 4000, Memory: 1024},
		Allocatable:      types.Resources{CPU: 3000, Memory: 512},
	})
	if err != nil {
		t.Fatalf("register node %s: %v", name, err)
	}
	return registered
}

func createDrainTestService(t *testing.T, ctx context.Context, service *MemoryService, replicas int) types.Service {
	t.Helper()
	created, err := service.CreateService(ctx, types.ServiceSpec{
		Name:                 "api",
		Image:                "nginx:1.27",
		Replicas:             replicas,
		ResourceRequirements: types.ResourceRequirements{},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return created
}

func singleTaskOnNode(t *testing.T, ctx context.Context, service *MemoryService, serviceID types.ServiceID, nodeID types.NodeID) types.Task {
	t.Helper()
	tasks, err := service.ListTasks(ctx, TaskFilter{ServiceID: serviceID, NodeID: nodeID})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	active := make([]types.Task, 0)
	for _, task := range tasks {
		if types.IsActiveTask(task) {
			active = append(active, task)
		}
	}
	if len(active) != 1 {
		t.Fatalf("expected one active task on node %s, got %#v", nodeID, tasks)
	}
	return active[0]
}
