package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

var errFakeEvent = errors.New("event failed")

type schedulerMetricsRecorder struct {
	runs      int
	errors    int
	attempts  int
	failures  int
	claimed   int
	conflicts int
	durations int
}

func (m *schedulerMetricsRecorder) IncSchedulerRuns() {
	m.runs++
}

func (m *schedulerMetricsRecorder) IncSchedulerErrors() {
	m.errors++
}

func (m *schedulerMetricsRecorder) ObserveSchedulerDuration(time.Duration) {
	m.durations++
}

func (m *schedulerMetricsRecorder) IncSchedulingAttempts() {
	m.attempts++
}

func (m *schedulerMetricsRecorder) IncSchedulingFailures() {
	m.failures++
}

func (m *schedulerMetricsRecorder) IncTasksClaimed() {
	m.claimed++
}

func (m *schedulerMetricsRecorder) IncAssignmentConflicts() {
	m.conflicts++
}

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
	assignedAt := time.Date(2026, 6, 11, 11, 0, 0, 0, time.UTC)
	store := &fakeStore{
		pending: []types.Task{task},
		nodes: []types.Node{
			nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
		},
		services: map[types.ServiceID]types.Service{
			"svc": serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, nil),
		},
	}
	metrics := &schedulerMetricsRecorder{}
	scheduler := New(store, WithMetrics(metrics))
	scheduler.now = func() time.Time { return assignedAt }

	assignments, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run scheduler: %v", err)
	}
	want := []Assignment{{
		TaskID:            "task-a",
		NodeID:            "node-a",
		AssignedAt:        assignedAt,
		ExpectedUpdatedAt: task.UpdatedAt,
	}}
	if !assignmentsEqual(assignments, want) {
		t.Fatalf("unexpected assignments %#v", assignments)
	}
	if len(store.assigned) != 1 || store.assigned[0].TaskID != assignments[0].TaskID ||
		store.assigned[0].NodeID != assignments[0].NodeID ||
		!store.assigned[0].ExpectedUpdatedAt.Equal(task.UpdatedAt) {
		t.Fatalf("expected assignment persisted, got %#v", store.assigned)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one event, got %d", len(store.events))
	}
	if store.events[0].Type != "task.assigned" || store.events[0].RelatedObjectID != "task-a" {
		t.Fatalf("unexpected event %#v", store.events[0])
	}
	if !store.events[0].Timestamp.Equal(assignedAt) {
		t.Fatalf("expected event assigned timestamp %s, got %s", assignedAt, store.events[0].Timestamp)
	}
	if metrics.runs != 1 || metrics.attempts != 1 || metrics.claimed != 1 || metrics.conflicts != 0 || metrics.failures != 0 {
		t.Fatalf("unexpected scheduler metrics %#v", metrics)
	}
}

func TestRunOnceIgnoresEventEmissionFailure(t *testing.T) {
	task := pendingTask("task-a", "svc")
	task.UpdatedAt = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	assignedAt := time.Date(2026, 6, 11, 11, 0, 0, 0, time.UTC)
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
	scheduler := New(store)
	scheduler.now = func() time.Time { return assignedAt }

	assignments, err := scheduler.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run scheduler: %v", err)
	}
	want := []Assignment{{
		TaskID:            "task-a",
		NodeID:            "node-a",
		AssignedAt:        assignedAt,
		ExpectedUpdatedAt: task.UpdatedAt,
	}}
	if !assignmentsEqual(assignments, want) {
		t.Fatalf("unexpected assignments %#v", assignments)
	}
}

func TestRunOnceRollsBackTransactionalAssignmentWhenEventFails(t *testing.T) {
	task := pendingTask("task-a", "svc")
	task.UpdatedAt = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	base := &fakeStore{
		pending: []types.Task{task},
		nodes: []types.Node{
			nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
		},
		services: map[types.ServiceID]types.Service{
			"svc": serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, nil),
		},
		failEvents: true,
	}

	assignments, err := New(&transactionalFakeStore{fakeStore: base}).RunOnce(context.Background())
	if !errors.Is(err, errFakeEvent) {
		t.Fatalf("expected transactional event failure, got %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected no returned assignments after rollback, got %#v", assignments)
	}
	if len(base.assigned) != 0 {
		t.Fatalf("expected assignment rollback, got %#v", base.assigned)
	}
	if len(base.events) != 0 {
		t.Fatalf("expected no persisted event, got %#v", base.events)
	}
}

func TestRunOnceSkipsAssignmentConflict(t *testing.T) {
	task := pendingTask("task-a", "svc")
	task.UpdatedAt = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	fake := &fakeStore{
		pending: []types.Task{task},
		nodes: []types.Node{
			nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
		},
		services: map[types.ServiceID]types.Service{
			"svc": serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, nil),
		},
		assignErr: store.ErrConflict,
	}

	metrics := &schedulerMetricsRecorder{}
	assignments, err := New(fake, WithMetrics(metrics)).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run scheduler: %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected conflicted assignment not to be returned, got %#v", assignments)
	}
	if len(fake.events) != 0 {
		t.Fatalf("expected no assignment event for conflicted task, got %#v", fake.events)
	}
	if metrics.attempts != 1 || metrics.conflicts != 1 || metrics.claimed != 0 || metrics.failures != 0 {
		t.Fatalf("unexpected scheduler metrics %#v", metrics)
	}
}

func TestRunOnceRecordsSchedulingFailure(t *testing.T) {
	metrics := &schedulerMetricsRecorder{}
	_, err := New(nil, WithMetrics(metrics)).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected scheduler error")
	}
	if metrics.attempts != 1 || metrics.errors != 1 || metrics.failures != 1 || metrics.claimed != 0 || metrics.conflicts != 0 {
		t.Fatalf("unexpected scheduler metrics %#v", metrics)
	}
}

func TestConcurrentSchedulerAttemptsAssignTaskOnce(t *testing.T) {
	task := pendingTask("task-a", "svc")
	task.UpdatedAt = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	store := &concurrentAssignmentStore{
		task:    task,
		nodes:   []types.Node{nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048})},
		service: serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, nil),
	}
	scheduler := New(store)
	assignment := Assignment{TaskID: task.ID, NodeID: "node-a"}

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, assigned, err := scheduler.persistAssignment(context.Background(), task, assignment)
			results <- assigned
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	successes := 0
	for assigned := range results {
		if assigned {
			successes++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("expected conflict to be treated as benign, got %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one assignment success, got %d", successes)
	}
	if store.task.ActualStatus != types.TaskAssigned || store.task.NodeID != "node-a" {
		t.Fatalf("expected task assigned once, got %#v", store.task)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one assignment event, got %#v", store.events)
	}
}

func TestConcurrentRunOnceAssignsTaskOnceAcrossSchedulers(t *testing.T) {
	task := pendingTask("task-a", "svc")
	task.UpdatedAt = time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	base := &concurrentAssignmentStore{
		task: task,
		nodes: []types.Node{
			nodeFixture("node-a", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
			nodeFixture("node-b", types.NodeReady, nil, types.Resources{CPU: 2000, Memory: 2048}),
		},
		service: serviceFixture("svc", types.Resources{CPU: 500, Memory: 512}, nil),
	}
	store := &concurrentRunOnceStore{
		concurrentAssignmentStore: base,
		pendingListed:             make(chan struct{}, 2),
		releasePending:            make(chan struct{}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan []Assignment, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assignments, err := New(store).RunOnce(ctx)
			results <- assignments
			errs <- err
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-store.pendingListed:
		case <-ctx.Done():
			t.Fatalf("waiting for concurrent scheduler list: %v", ctx.Err())
		}
	}
	close(store.releasePending)
	wg.Wait()
	close(results)
	close(errs)

	successes := 0
	for assignments := range results {
		if len(assignments) > 1 {
			t.Fatalf("expected each scheduler to return at most one assignment, got %#v", assignments)
		}
		successes += len(assignments)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("expected concurrent conflicts to be benign, got %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one scheduler to claim the task, got %d", successes)
	}
	if base.task.ActualStatus != types.TaskAssigned || base.task.NodeID == "" {
		t.Fatalf("expected task assigned once, got %#v", base.task)
	}
	if len(base.events) != 1 {
		t.Fatalf("expected one assignment event, got %#v", base.events)
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
	assignErr  error
}

type transactionalFakeStore struct {
	*fakeStore
}

func (s *transactionalFakeStore) WithTx(ctx context.Context, fn store.TxFunc) error {
	assigned := append([]Assignment(nil), s.assigned...)
	events := append([]types.Event(nil), s.events...)
	if err := fn(ctx, s); err != nil {
		s.assigned = assigned
		s.events = events
		return err
	}
	return nil
}

type concurrentAssignmentStore struct {
	mu      sync.Mutex
	task    types.Task
	nodes   []types.Node
	service types.Service
	events  []types.Event
}

type concurrentRunOnceStore struct {
	*concurrentAssignmentStore
	pendingListed  chan struct{}
	releasePending chan struct{}
}

func (s *concurrentAssignmentStore) WithTx(ctx context.Context, fn store.TxFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := s.task
	events := append([]types.Event(nil), s.events...)
	if err := fn(ctx, s); err != nil {
		s.task = task
		s.events = events
		return err
	}
	return nil
}

func (s *concurrentAssignmentStore) ListTasksByStatus(_ context.Context, status types.TaskStatus) ([]types.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task.ActualStatus == status {
		return []types.Task{s.task}, nil
	}
	return nil, nil
}

func (s *concurrentRunOnceStore) ListTasksByStatus(ctx context.Context, status types.TaskStatus) ([]types.Task, error) {
	tasks, err := s.concurrentAssignmentStore.ListTasksByStatus(ctx, status)
	if status == types.TaskPending {
		s.pendingListed <- struct{}{}
		select {
		case <-s.releasePending:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return tasks, err
}

func (s *concurrentAssignmentStore) ListNodesByStatus(_ context.Context, status types.NodeStatus) ([]types.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var nodes []types.Node
	for _, node := range s.nodes {
		if node.Status == status {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (s *concurrentAssignmentStore) ListTasksByNode(context.Context, types.NodeID) ([]types.Task, error) {
	return nil, nil
}

func (s *concurrentAssignmentStore) GetService(context.Context, types.ServiceID) (types.Service, error) {
	return s.service, nil
}

func (s *concurrentAssignmentStore) AssignTask(_ context.Context, id types.TaskID, nodeID types.NodeID, expectedUpdatedAt time.Time) (types.Task, error) {
	if s.task.ID != id {
		return types.Task{}, store.ErrNotFound
	}
	if s.task.ActualStatus != types.TaskPending || !s.task.UpdatedAt.Equal(expectedUpdatedAt) {
		return types.Task{}, store.ErrConflict
	}
	s.task.NodeID = nodeID
	s.task.ActualStatus = types.TaskAssigned
	s.task.UpdatedAt = s.task.UpdatedAt.Add(time.Second)
	return s.task, nil
}

func (s *concurrentAssignmentStore) AppendEvent(_ context.Context, event types.Event) (types.Event, error) {
	s.events = append(s.events, event)
	return event, nil
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

func (s *fakeStore) AssignTask(_ context.Context, id types.TaskID, nodeID types.NodeID, expectedUpdatedAt time.Time) (types.Task, error) {
	if s.assignErr != nil {
		return types.Task{}, s.assignErr
	}
	s.assigned = append(s.assigned, Assignment{
		TaskID:            id,
		NodeID:            nodeID,
		ExpectedUpdatedAt: expectedUpdatedAt.UTC(),
	})
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
