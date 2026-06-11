package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

var errFakeEvent = errors.New("event failed")

func TestPlanAssignments(t *testing.T) {
	service := serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, nil)
	labeledService := serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, []types.PlacementConstraint{
		{Key: "role", Operator: types.ConstraintEquals, Value: "app"},
	})

	tests := []struct {
		name     string
		tasks    []types.Task
		nodes    []types.Node
		running  []types.Task
		services map[types.ServiceID]types.Service
		want     []Assignment
	}{
		{
			name:     "no nodes",
			tasks:    []types.Task{pendingTask("task-a", "svc")},
			services: map[types.ServiceID]types.Service{"svc": service},
		},
		{
			name:  "labels mismatch",
			tasks: []types.Task{pendingTask("task-a", "svc")},
			nodes: []types.Node{
				nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
			},
			services: map[types.ServiceID]types.Service{"svc": labeledService},
		},
		{
			name:  "insufficient resources",
			tasks: []types.Task{pendingTask("task-a", "svc")},
			nodes: []types.Node{
				nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 250, Memory: 2048}),
			},
			services: map[types.ServiceID]types.Service{"svc": service},
		},
		{
			name:  "spread across nodes",
			tasks: []types.Task{pendingTask("task-b", "svc"), pendingTask("task-a", "svc")},
			nodes: []types.Node{
				nodeFixture("node-b", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
				nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
			},
			services: map[types.ServiceID]types.Service{"svc": service},
			want: []Assignment{
				{TaskID: "task-a", NodeID: "node-a"},
				{TaskID: "task-b", NodeID: "node-b"},
			},
		},
		{
			name:  "deterministic tie breaking",
			tasks: []types.Task{pendingTask("task-a", "svc")},
			nodes: []types.Node{
				nodeFixture("node-b", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
				nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
			},
			services: map[types.ServiceID]types.Service{"svc": service},
			want:     []Assignment{{TaskID: "task-a", NodeID: "node-a"}},
		},
		{
			name:  "node draining",
			tasks: []types.Task{pendingTask("task-a", "svc")},
			nodes: []types.Node{
				nodeFixture("node-a", types.NodeDraining, nil, types.Resources{CPU: 2000, Memory: 2048}),
			},
			services: map[types.ServiceID]types.Service{"svc": service},
		},
		{
			name:  "prefer more free memory",
			tasks: []types.Task{pendingTask("task-a", "svc")},
			nodes: []types.Node{
				nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
				nodeFixture("node-b", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 4096}),
			},
			services: map[types.ServiceID]types.Service{"svc": service},
			want:     []Assignment{{TaskID: "task-a", NodeID: "node-b"}},
		},
		{
			name:  "prefer fewer running tasks for same service",
			tasks: []types.Task{pendingTask("task-a", "svc")},
			nodes: []types.Node{
				nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
				nodeFixture("node-b", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
			},
			running: []types.Task{
				runningTask("running-a", "svc", "node-a"),
				runningTask("running-b", "other", "node-b"),
			},
			services: map[types.ServiceID]types.Service{
				"svc":   service,
				"other": serviceFixture("other", types.Resources{CPU: 500, Memory: 512}, nil),
			},
			want: []Assignment{{TaskID: "task-a", NodeID: "node-b"}},
		},
		{
			name:  "prefer fewer total running tasks",
			tasks: []types.Task{pendingTask("task-a", "svc")},
			nodes: []types.Node{
				nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
				nodeFixture("node-b", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
			},
			running: []types.Task{runningTask("running-a", "other", "node-a")},
			services: map[types.ServiceID]types.Service{
				"svc":   service,
				"other": serviceFixture("other", types.Resources{}, nil),
			},
			want: []Assignment{{TaskID: "task-a", NodeID: "node-b"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Plan(PlanInput{
				PendingTasks: tt.tasks,
				Nodes:        tt.nodes,
				RunningTasks: tt.running,
				Services:     tt.services,
			})
			if !assignmentsEqual(got, tt.want) {
				t.Fatalf("expected assignments %#v, got %#v", tt.want, got)
			}
		})
	}
}

func TestRunOnceAssignsTasksAndEmitsEvents(t *testing.T) {
	task := pendingTask("task-a", "svc")
	task.UpdatedAt = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{
		pending: []types.Task{task},
		nodes: []types.Node{
			nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
		},
		services: map[types.ServiceID]types.Service{
			"svc": serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, nil),
		},
	}
	scheduler := New(store)
	scheduler.now = func() time.Time { return time.Date(2026, 6, 11, 11, 0, 0, 0, time.UTC) }

	assignments, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run scheduler: %v", err)
	}
	if !assignmentsEqual(assignments, []Assignment{{TaskID: "task-a", NodeID: "node-a"}}) {
		t.Fatalf("unexpected assignments %#v", assignments)
	}
	if len(store.assigned) != 1 || store.assigned[0] != assignments[0] {
		t.Fatalf("expected assignment persisted, got %#v", store.assigned)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one event, got %d", len(store.events))
	}
	if store.events[0].Type != "task.assigned" || store.events[0].RelatedObjectID != "task-a" {
		t.Fatalf("unexpected event %#v", store.events[0])
	}
}

func TestRunOnceIgnoresEventEmissionFailure(t *testing.T) {
	task := pendingTask("task-a", "svc")
	task.UpdatedAt = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{
		pending: []types.Task{task},
		nodes: []types.Node{
			nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
		},
		services: map[types.ServiceID]types.Service{
			"svc": serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, nil),
		},
		failEvents: true,
	}

	assignments, err := New(store).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run scheduler: %v", err)
	}
	if !assignmentsEqual(assignments, []Assignment{{TaskID: "task-a", NodeID: "node-a"}}) {
		t.Fatalf("unexpected assignments %#v", assignments)
	}
}

func TestRunOnceAccountsForNonTerminalAssignedTasks(t *testing.T) {
	task := pendingTask("task-a", "svc")
	task.UpdatedAt = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{
		pending: []types.Task{task},
		nodes: []types.Node{
			nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 1000, Memory: 1024}),
		},
		byNode: map[types.NodeID][]types.Task{
			"node-a": {
				{
					ID:            "existing",
					ServiceID:     "other",
					NodeID:        "node-a",
					DesiredStatus: types.TaskRunning,
					ActualStatus:  types.TaskAssigned,
				},
			},
		},
		services: map[types.ServiceID]types.Service{
			"svc":   serviceFixture("svc", types.Resources{CPU: 400, Memory: 512}, nil),
			"other": serviceFixture("other", types.Resources{CPU: 700, Memory: 512}, nil),
		},
	}

	assignments, err := New(store).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run scheduler: %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected no assignment because assigned task consumes resources, got %#v", assignments)
	}
}

type fakeStore struct {
	pending    []types.Task
	nodes      []types.Node
	byNode     map[types.NodeID][]types.Task
	services   map[types.ServiceID]types.Service
	assigned   []Assignment
	events     []types.Event
	failEvents bool
}

func (s *fakeStore) ListTasksByStatus(_ context.Context, status types.TaskStatus) ([]types.Task, error) {
	if status == types.TaskPending {
		return s.pending, nil
	}
	return nil, nil
}

func (s *fakeStore) ListNodesByStatus(_ context.Context, status types.NodeStatus) ([]types.Node, error) {
	var nodes []types.Node
	for _, node := range s.nodes {
		if node.Status == status {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (s *fakeStore) ListTasksByNode(_ context.Context, nodeID types.NodeID) ([]types.Task, error) {
	return s.byNode[nodeID], nil
}

func (s *fakeStore) GetService(_ context.Context, id types.ServiceID) (types.Service, error) {
	return s.services[id], nil
}

func (s *fakeStore) AssignTask(_ context.Context, id types.TaskID, nodeID types.NodeID, _ time.Time) (types.Task, error) {
	s.assigned = append(s.assigned, Assignment{TaskID: id, NodeID: nodeID})
	return types.Task{ID: id, NodeID: nodeID, ActualStatus: types.TaskAssigned}, nil
}

func (s *fakeStore) AppendEvent(_ context.Context, event types.Event) (types.Event, error) {
	if s.failEvents {
		return types.Event{}, errFakeEvent
	}
	s.events = append(s.events, event)
	return event, nil
}

func pendingTask(id types.TaskID, serviceID types.ServiceID) types.Task {
	return types.Task{
		ID:            id,
		ServiceID:     serviceID,
		DesiredStatus: types.TaskRunning,
		ActualStatus:  types.TaskPending,
		Image:         "nginx:1.27",
		Version:       1,
	}
}

func runningTask(id types.TaskID, serviceID types.ServiceID, nodeID types.NodeID) types.Task {
	task := pendingTask(id, serviceID)
	task.NodeID = nodeID
	task.ActualStatus = types.TaskRunning
	return task
}

func nodeFixture(id types.NodeID, status types.NodeStatus, labels map[string]string, allocatable types.Resources) types.Node {
	return types.Node{
		ID:          id,
		Labels:      labels,
		Capacity:    allocatable,
		Allocatable: allocatable,
		Status:      status,
	}
}

func serviceFixture(id types.ServiceID, requests types.Resources, constraints []types.PlacementConstraint) types.Service {
	return types.Service{
		ID: id,
		Spec: types.ServiceSpec{
			Name:                 string(id),
			Image:                "nginx:1.27",
			Replicas:             1,
			ResourceRequirements: types.ResourceRequirements{Requests: requests},
			PlacementConstraints: constraints,
		},
		DeploymentVersion: 1,
	}
}

func assignmentsEqual(left, right []Assignment) bool {
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
