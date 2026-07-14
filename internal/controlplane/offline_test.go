package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestMarkStaleNodesOfflineReplacesStatelessTaskOnce(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	current := base
	service.now = func() time.Time { return current }

	nodeA := registerOfflineTestNode(t, ctx, service, "node-a")
	created := createOfflineTestService(t, ctx, service, "api", false)
	oldTask := singleTaskOnNode(t, ctx, service, created.ID, nodeA.Node.ID)

	current = base.Add(25 * time.Second)
	nodeB := registerOfflineTestNode(t, ctx, service, "node-b")

	current = base.Add(40 * time.Second)
	offline, err := service.MarkStaleNodesOffline(ctx, 30*time.Second)
	if err != nil {
		t.Fatalf("mark stale nodes offline: %v", err)
	}
	if len(offline) != 1 || offline[0].ID != nodeA.Node.ID || offline[0].Status != types.NodeOffline {
		t.Fatalf("unexpected offline nodes %#v", offline)
	}

	lost, err := service.GetTask(ctx, oldTask.ID)
	if err != nil {
		t.Fatalf("get lost task: %v", err)
	}
	if lost.DesiredStatus != types.TaskRemoved || lost.ActualStatus != types.TaskFailed || lost.FailureReason != "node_lost" {
		t.Fatalf("expected lost task failed/removed, got %#v", lost)
	}
	if !hasTaskCondition(lost, types.TaskConditionNodeLost) {
		t.Fatalf("expected node_lost condition, got %#v", lost.Conditions)
	}

	replacement := singleTaskOnNode(t, ctx, service, created.ID, nodeB.Node.ID)
	if replacement.ID == oldTask.ID {
		t.Fatalf("expected replacement on node-b, got old task")
	}

	before := taskCountForService(t, ctx, service, created.ID)
	offline, err = service.MarkStaleNodesOffline(ctx, 30*time.Second)
	if err != nil {
		t.Fatalf("mark stale nodes offline again: %v", err)
	}
	if len(offline) != 0 {
		t.Fatalf("expected no newly offline nodes, got %#v", offline)
	}
	after := taskCountForService(t, ctx, service, created.ID)
	if after != before {
		t.Fatalf("expected no duplicate replacement, before=%d after=%d", before, after)
	}

	items, err := service.ListEvents(ctx, events.Filter{NodeID: nodeA.Node.ID, Type: events.TypeNodeOfflineDetected})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one offline event, got %#v", items)
	}
}

func TestMarkStaleNodesOfflineDoesNotReplaceStatefulTask(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	current := base
	service.now = func() time.Time { return current }

	nodeA := registerOfflineTestNode(t, ctx, service, "node-a")
	created := createOfflineTestService(t, ctx, service, "db", true)
	oldTask := singleTaskOnNode(t, ctx, service, created.ID, nodeA.Node.ID)

	current = base.Add(25 * time.Second)
	nodeB := registerOfflineTestNode(t, ctx, service, "node-b")

	current = base.Add(40 * time.Second)
	if _, err := service.MarkStaleNodesOffline(ctx, 30*time.Second); err != nil {
		t.Fatalf("mark stale nodes offline: %v", err)
	}

	lost, err := service.GetTask(ctx, oldTask.ID)
	if err != nil {
		t.Fatalf("get lost task: %v", err)
	}
	if lost.DesiredStatus != types.TaskRunning || lost.ActualStatus != types.TaskAssigned {
		t.Fatalf("expected stateful task to stay assigned for manual recovery, got %#v", lost)
	}
	if !hasTaskCondition(lost, types.TaskConditionNodeLost) {
		t.Fatalf("expected node_lost condition, got %#v", lost.Conditions)
	}
	tasks, err := service.ListTasks(ctx, TaskFilter{ServiceID: created.ID, NodeID: nodeB.Node.ID})
	if err != nil {
		t.Fatalf("list replacement tasks: %v", err)
	}
	for _, task := range tasks {
		if types.IsActiveTask(task) {
			t.Fatalf("expected no stateful replacement on node-b, got %#v", tasks)
		}
	}
}

func TestHeartbeatReturnsOfflineNodeReady(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	current := base
	service.now = func() time.Time { return current }

	nodeA := registerOfflineTestNode(t, ctx, service, "node-a")
	created := createOfflineTestService(t, ctx, service, "api", false)
	current = base.Add(25 * time.Second)
	registerOfflineTestNode(t, ctx, service, "node-b")

	current = base.Add(40 * time.Second)
	if _, err := service.MarkStaleNodesOffline(ctx, 30*time.Second); err != nil {
		t.Fatalf("mark stale nodes offline: %v", err)
	}
	current = base.Add(41 * time.Second)
	command, err := service.HeartbeatNode(ctx, NodeHeartbeat{NodeID: nodeA.Node.ID})
	if err != nil {
		t.Fatalf("heartbeat returned node: %v", err)
	}
	if command.Node.Status != types.NodeReady {
		t.Fatalf("expected returned node ready, got %q", command.Node.Status)
	}
	assigned, err := service.ListAssignedTasks(ctx, nodeA.Node.ID)
	if err != nil {
		t.Fatalf("list assigned tasks: %v", err)
	}
	for _, task := range assigned {
		if task.Task.ServiceID == created.ID {
			t.Fatalf("expected removed node_lost task not to be reassigned to returned node, got %#v", assigned)
		}
	}
}

func registerOfflineTestNode(t *testing.T, ctx context.Context, service *MemoryService, name string) NodeCommand {
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

func createOfflineTestService(t *testing.T, ctx context.Context, service *MemoryService, name string, stateful bool) types.Service {
	t.Helper()
	created, err := service.CreateService(ctx, types.ServiceSpec{
		Name:                 name,
		Image:                "nginx:1.27",
		Stateful:             stateful,
		Replicas:             1,
		ResourceRequirements: types.ResourceRequirements{},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return created
}

func hasTaskCondition(task types.Task, conditionType types.TaskConditionType) bool {
	for _, condition := range task.Conditions {
		if condition.Type == conditionType {
			return true
		}
	}
	return false
}

func taskCountForService(t *testing.T, ctx context.Context, service *MemoryService, serviceID types.ServiceID) int {
	t.Helper()
	tasks, err := service.ListTasks(ctx, TaskFilter{ServiceID: serviceID})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	return len(tasks)
}
