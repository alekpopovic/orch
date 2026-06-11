package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

var errFakeEvent = errors.New("event failed")

func TestReconcileOnceServiceReplicas(t *testing.T) {
	tests := []struct {
		name        string
		services    []types.Service
		tasks       []types.Task
		wantCreated int
		wantStopped []types.TaskID
		wantEvents  int
	}{
		{
			name: "scale up",
			services: []types.Service{
				serviceFixture("svc", 3, 1, types.RestartPolicy{}),
			},
			tasks: []types.Task{
				taskFixture("task-a", "svc", 1, types.TaskRunning, types.TaskRunning),
			},
			wantCreated: 2,
			wantEvents:  2,
		},
		{
			name: "scale down",
			services: []types.Service{
				serviceFixture("svc", 1, 1, types.RestartPolicy{}),
			},
			tasks: []types.Task{
				taskFixture("task-a", "svc", 1, types.TaskRunning, types.TaskRunning),
				taskFixture("task-b", "svc", 1, types.TaskRunning, types.TaskRunning),
				taskFixture("task-c", "svc", 1, types.TaskRunning, types.TaskRunning),
			},
			wantStopped: []types.TaskID{"task-c", "task-b"},
			wantEvents:  2,
		},
		{
			name: "failed task replacement",
			services: []types.Service{
				serviceFixture("svc", 2, 1, types.RestartPolicy{Condition: types.RestartOnFailure}),
			},
			tasks: []types.Task{
				taskFixture("task-a", "svc", 1, types.TaskRunning, types.TaskRunning),
				taskFixture("task-b", "svc", 1, types.TaskRunning, types.TaskFailed),
			},
			wantCreated: 1,
			wantEvents:  1,
		},
		{
			name: "no-op when desired equals actual",
			services: []types.Service{
				serviceFixture("svc", 2, 1, types.RestartPolicy{}),
			},
			tasks: []types.Task{
				taskFixture("task-a", "svc", 1, types.TaskRunning, types.TaskRunning),
				taskFixture("task-b", "svc", 1, types.TaskRunning, types.TaskPending),
			},
		},
		{
			name: "service deletion",
			tasks: []types.Task{
				taskFixture("task-a", "deleted-svc", 1, types.TaskRunning, types.TaskRunning),
			},
			wantStopped: []types.TaskID{"task-a"},
			wantEvents:  1,
		},
		{
			name: "version mismatch",
			services: []types.Service{
				serviceFixture("svc", 1, 2, types.RestartPolicy{}),
			},
			tasks: []types.Task{
				taskFixture("task-a", "svc", 1, types.TaskRunning, types.TaskRunning),
			},
			wantCreated: 1,
			wantStopped: []types.TaskID{"task-a"},
			wantEvents:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore(tt.services, tt.tasks)
			metrics := &fakeMetrics{}
			r := New(store, WithMetrics(metrics))
			r.now = func() time.Time { return time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC) }

			if err := r.ReconcileOnce(context.Background()); err != nil {
				t.Fatalf("reconcile once: %v", err)
			}
			if got := len(store.created); got != tt.wantCreated {
				t.Fatalf("expected %d created tasks, got %d", tt.wantCreated, got)
			}
			if !taskIDsEqual(store.stopped, tt.wantStopped) {
				t.Fatalf("expected stopped tasks %#v, got %#v", tt.wantStopped, store.stopped)
			}
			if got := len(store.events); got != tt.wantEvents {
				t.Fatalf("expected %d events, got %d", tt.wantEvents, got)
			}
			if metrics.created != tt.wantCreated {
				t.Fatalf("expected created metric %d, got %d", tt.wantCreated, metrics.created)
			}
			if metrics.stopped != len(tt.wantStopped) {
				t.Fatalf("expected stopped metric %d, got %d", len(tt.wantStopped), metrics.stopped)
			}
			if metrics.durations != 1 {
				t.Fatalf("expected duration metric once, got %d", metrics.durations)
			}
		})
	}
}

func TestReconcileOnceIsIdempotent(t *testing.T) {
	store := newFakeStore(
		[]types.Service{serviceFixture("svc", 2, 1, types.RestartPolicy{})},
		nil,
	)
	r := New(store)

	if err := r.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if err := r.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if got := len(store.created); got != 2 {
		t.Fatalf("expected exactly 2 created tasks after two runs, got %d", got)
	}
}

func TestReconcileOnceUsesLeaderLock(t *testing.T) {
	lock := &fakeLeaderLock{}
	store := newFakeStore(nil, nil)
	r := New(store, WithLeaderLock(lock))

	if err := r.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile once: %v", err)
	}
	if lock.acquired != 1 || lock.released != 1 {
		t.Fatalf("expected lock acquire/release once, got acquired=%d released=%d", lock.acquired, lock.released)
	}
}

func TestReconcileOnceIgnoresEventEmissionFailure(t *testing.T) {
	store := newFakeStore(
		[]types.Service{serviceFixture("svc", 1, 1, types.RestartPolicy{})},
		nil,
	)
	store.failEvents = true
	if err := New(store).ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile once: %v", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected task to be created despite event failure")
	}
}

func TestReconcileDeletingServiceWithRunningTasks(t *testing.T) {
	store := newFakeStore(
		[]types.Service{serviceFixtureWithStatus("svc", 1, 1, types.RestartPolicy{}, types.ServiceDeleting)},
		[]types.Task{taskFixture("task-a", "svc", 1, types.TaskRunning, types.TaskRunning)},
	)

	if err := New(store).ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile once: %v", err)
	}
	if !taskIDsEqual(store.stopped, []types.TaskID{"task-a"}) {
		t.Fatalf("expected stop directive for task-a, got %#v", store.stopped)
	}
	if store.services["svc"].Status != types.ServiceDeleting {
		t.Fatalf("expected service to remain deleting while task is not removed, got %q", store.services["svc"].Status)
	}
}

func TestReconcileDeletingServiceFinalCleanupAfterAgentReturns(t *testing.T) {
	store := newFakeStore(
		[]types.Service{serviceFixtureWithStatus("svc", 1, 1, types.RestartPolicy{}, types.ServiceDeleting)},
		[]types.Task{taskFixture("task-a", "svc", 1, types.TaskStopped, types.TaskRemoved)},
	)

	if err := New(store).ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("reconcile once: %v", err)
	}
	if store.services["svc"].Status != types.ServiceDeleted {
		t.Fatalf("expected service deleted after all tasks removed, got %q", store.services["svc"].Status)
	}
	if got := len(store.serviceStatusUpdates); got != 1 {
		t.Fatalf("expected one service status update, got %d", got)
	}
}

type fakeStore struct {
	services             map[types.ServiceID]types.Service
	tasks                map[types.TaskID]types.Task
	created              []types.Task
	stopped              []types.TaskID
	serviceStatusUpdates []types.ServiceStatus
	events               []types.Event
	nextID               int
	failEvents           bool
}

func newFakeStore(services []types.Service, tasks []types.Task) *fakeStore {
	store := &fakeStore{
		services: make(map[types.ServiceID]types.Service),
		tasks:    make(map[types.TaskID]types.Task),
		nextID:   1,
	}
	for _, service := range services {
		store.services[service.ID] = service
	}
	for _, task := range tasks {
		store.tasks[task.ID] = task
	}
	return store
}

func (s *fakeStore) ListServices(context.Context) ([]types.Service, error) {
	services := make([]types.Service, 0, len(s.services))
	for _, service := range s.services {
		services = append(services, service)
	}
	return services, nil
}

func (s *fakeStore) ListTasksByService(_ context.Context, serviceID types.ServiceID) ([]types.Task, error) {
	tasks := make([]types.Task, 0)
	for _, task := range s.tasks {
		if task.ServiceID == serviceID {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (s *fakeStore) ListTasksByStatus(_ context.Context, status types.TaskStatus) ([]types.Task, error) {
	tasks := make([]types.Task, 0)
	for _, task := range s.tasks {
		if task.ActualStatus == status {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (s *fakeStore) CreateTask(_ context.Context, task types.Task) (types.Task, error) {
	task.ID = types.TaskID(fmt.Sprintf("created-task-%d", s.nextID))
	s.nextID++
	task.CreatedAt = time.Now().UTC()
	task.UpdatedAt = task.CreatedAt
	s.tasks[task.ID] = task
	s.created = append(s.created, task)
	return task, nil
}

func (s *fakeStore) StopTask(_ context.Context, id types.TaskID, _ time.Time) (types.Task, error) {
	task := s.tasks[id]
	if task.DesiredStatus == types.TaskStopped || task.DesiredStatus == types.TaskRemoved {
		return task, nil
	}
	task.DesiredStatus = types.TaskStopped
	task.UpdatedAt = time.Now().UTC()
	s.tasks[id] = task
	s.stopped = append(s.stopped, id)
	return task, nil
}

func (s *fakeStore) UpdateServiceStatus(_ context.Context, id types.ServiceID, status types.ServiceStatus, _ time.Time) (types.Service, error) {
	service, ok := s.services[id]
	if !ok {
		return types.Service{}, fmt.Errorf("service not found")
	}
	service.Status = status
	service.UpdatedAt = time.Now().UTC()
	s.services[id] = service
	s.serviceStatusUpdates = append(s.serviceStatusUpdates, status)
	return service, nil
}

func (s *fakeStore) AppendEvent(_ context.Context, event types.Event) (types.Event, error) {
	if s.failEvents {
		return types.Event{}, errFakeEvent
	}
	s.events = append(s.events, event)
	return event, nil
}

type fakeMetrics struct {
	runs      int
	durations int
	errors    int
	created   int
	stopped   int
}

func (m *fakeMetrics) IncReconciliationRuns()                      { m.runs++ }
func (m *fakeMetrics) ObserveReconciliationDuration(time.Duration) { m.durations++ }
func (m *fakeMetrics) IncReconciliationErrors()                    { m.errors++ }
func (m *fakeMetrics) AddCreatedTasks(count int)                   { m.created += count }
func (m *fakeMetrics) AddStoppedTasks(count int)                   { m.stopped += count }

type fakeLeaderLock struct {
	acquired int
	released int
}

func (l *fakeLeaderLock) Acquire(context.Context) (Lease, error) {
	l.acquired++
	return fakeLease{lock: l}, nil
}

type fakeLease struct {
	lock *fakeLeaderLock
}

func (l fakeLease) Release(context.Context) error {
	l.lock.released++
	return nil
}

func serviceFixture(id types.ServiceID, replicas int, version int64, policy types.RestartPolicy) types.Service {
	return serviceFixtureWithStatus(id, replicas, version, policy, types.ServiceActive)
}

func serviceFixtureWithStatus(id types.ServiceID, replicas int, version int64, policy types.RestartPolicy, status types.ServiceStatus) types.Service {
	return types.Service{
		ID: id,
		Spec: types.ServiceSpec{
			Name:          string(id),
			Image:         "nginx:1.27",
			Replicas:      replicas,
			RestartPolicy: policy,
		},
		Status:            status,
		DeploymentVersion: version,
	}
}

func taskFixture(id types.TaskID, serviceID types.ServiceID, version int64, desired types.TaskStatus, actual types.TaskStatus) types.Task {
	return types.Task{
		ID:            id,
		ServiceID:     serviceID,
		DesiredStatus: desired,
		ActualStatus:  actual,
		Image:         "nginx:1.27",
		Version:       version,
		UpdatedAt:     time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC),
	}
}

func taskIDsEqual(left, right []types.TaskID) bool {
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
