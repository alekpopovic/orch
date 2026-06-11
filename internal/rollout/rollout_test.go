package rollout

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestControllerSuccessfulRollout(t *testing.T) {
	ctx := context.Background()
	fake := newFakeStore(2, types.Deployment{MaxUnavailable: 1, MaxSurge: 1})
	controller := NewController(fake, slog.Default())

	mustRunOnce(t, controller, ctx)
	assertTaskVersions(t, fake.tasks, map[int64]int{1: 2, 2: 1})
	fake.markNewestNewTask(types.TaskHealthy)

	mustRunOnce(t, controller, ctx)
	assertTaskCounts(t, fake.tasks, 2, 1)

	mustRunOnce(t, controller, ctx)
	assertTaskVersions(t, fake.tasks, map[int64]int{1: 1, 2: 2})
	fake.markNewestNewTask(types.TaskHealthy)

	mustRunOnce(t, controller, ctx)
	if fake.deployment.Status != types.DeploymentSucceeded {
		t.Fatalf("expected succeeded rollout, got %q", fake.deployment.Status)
	}
	assertTaskCounts(t, fake.tasks, 2, 2)
}

func TestControllerFailsWhenNewTaskFails(t *testing.T) {
	ctx := context.Background()
	fake := newFakeStore(1, types.Deployment{MaxUnavailable: 1, MaxSurge: 1})
	fake.tasks["new-failed"] = task("new-failed", 2, types.TaskRunning, types.TaskFailed)

	if err := NewController(fake, slog.Default()).RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if fake.deployment.Status != types.DeploymentFailed {
		t.Fatalf("expected failed rollout, got %q", fake.deployment.Status)
	}
}

func TestControllerRespectsMaxUnavailable(t *testing.T) {
	ctx := context.Background()
	fake := newFakeStore(2, types.Deployment{MaxUnavailable: 0, MaxSurge: 1})
	fake.tasks["new-pending"] = task("new-pending", 2, types.TaskRunning, types.TaskPending)
	fake.deployment.Status = types.DeploymentRunning

	if err := NewController(fake, slog.Default()).RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
	assertTaskCounts(t, fake.tasks, 3, 0)
}

func TestControllerRespectsMaxSurge(t *testing.T) {
	ctx := context.Background()
	fake := newFakeStore(2, types.Deployment{MaxUnavailable: 1, MaxSurge: 1})

	if err := NewController(fake, slog.Default()).RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
	assertTaskVersions(t, fake.tasks, map[int64]int{1: 2, 2: 1})
}

func TestControllerIdempotentResume(t *testing.T) {
	ctx := context.Background()
	fake := newFakeStore(2, types.Deployment{MaxUnavailable: 1, MaxSurge: 1})
	fake.deployment.Status = types.DeploymentRunning
	fake.tasks["new-existing"] = task("new-existing", 2, types.TaskRunning, types.TaskPending)

	if err := NewController(fake, slog.Default()).RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
	assertTaskVersions(t, fake.tasks, map[int64]int{1: 2, 2: 1})
}

func TestControllerCompletesRollback(t *testing.T) {
	ctx := context.Background()
	fake := newFakeStore(2, types.Deployment{
		FromVersion:    2,
		ToVersion:      1,
		Status:         types.DeploymentRollingBack,
		MaxUnavailable: 1,
		MaxSurge:       0,
	})
	fake.service.DeploymentVersion = 1
	fake.service.Spec.Image = "ghcr.io/example/api:1.0.0"
	fake.tasks = map[types.TaskID]types.Task{
		"old-a": task("old-a", 2, types.TaskRunning, types.TaskHealthy),
		"old-b": task("old-b", 2, types.TaskRunning, types.TaskHealthy),
		"new-a": task("new-a", 1, types.TaskRunning, types.TaskHealthy),
		"new-b": task("new-b", 1, types.TaskRunning, types.TaskHealthy),
	}

	if err := NewController(fake, slog.Default()).RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if fake.deployment.Status != types.DeploymentRolledBack {
		t.Fatalf("expected rolled_back deployment, got %q", fake.deployment.Status)
	}
}

func mustRunOnce(t *testing.T, controller *Controller, ctx context.Context) {
	t.Helper()
	if err := controller.RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
}

type fakeStore struct {
	service    types.Service
	deployment types.Deployment
	tasks      map[types.TaskID]types.Task
	events     []types.Event
	nextTask   int
	now        time.Time
}

func newFakeStore(replicas int, deployment types.Deployment) *fakeStore {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	if deployment.ID == "" {
		deployment.ID = "00000000-0000-4000-8000-000000000100"
	}
	deployment.ServiceID = "00000000-0000-4000-8000-000000000001"
	if deployment.FromVersion == 0 {
		deployment.FromVersion = 1
	}
	if deployment.ToVersion == 0 {
		deployment.ToVersion = 2
	}
	deployment.Strategy = types.RolloutRollingUpdate
	if deployment.Status == "" {
		deployment.Status = types.DeploymentPending
	}
	deployment.CreatedAt = now
	deployment.UpdatedAt = now

	fake := &fakeStore{
		service: types.Service{
			ID: deployment.ServiceID,
			Spec: types.ServiceSpec{
				Name:     "api",
				Image:    "ghcr.io/example/api:2.0.0",
				Replicas: replicas,
			},
			DeploymentVersion: 2,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		deployment: deployment,
		tasks:      make(map[types.TaskID]types.Task),
		now:        now,
	}
	for i := 0; i < replicas; i++ {
		id := types.TaskID("old-" + string(rune('a'+i)))
		fake.tasks[id] = task(string(id), 1, types.TaskRunning, types.TaskHealthy)
	}
	return fake
}

func task(id string, version int64, desired types.TaskStatus, actual types.TaskStatus) types.Task {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	return types.Task{
		ID:            types.TaskID(id),
		ServiceID:     "00000000-0000-4000-8000-000000000001",
		DesiredStatus: desired,
		ActualStatus:  actual,
		Image:         "ghcr.io/example/api:2.0.0",
		Version:       version,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (s *fakeStore) GetService(context.Context, types.ServiceID) (types.Service, error) {
	return s.service, nil
}

func (s *fakeStore) ListTasksByService(context.Context, types.ServiceID) ([]types.Task, error) {
	tasks := make([]types.Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (s *fakeStore) CreateTask(_ context.Context, task types.Task) (types.Task, error) {
	s.nextTask++
	task.ID = types.TaskID("new-" + string(rune('a'+s.nextTask-1)))
	task.CreatedAt = s.now
	task.UpdatedAt = s.now
	s.tasks[task.ID] = task
	return task, nil
}

func (s *fakeStore) StopTask(_ context.Context, id types.TaskID, expectedUpdatedAt time.Time) (types.Task, error) {
	task, ok := s.tasks[id]
	if !ok {
		return types.Task{}, store.ErrNotFound
	}
	if !task.UpdatedAt.Equal(expectedUpdatedAt) {
		return types.Task{}, store.ErrConflict
	}
	task.DesiredStatus = types.TaskStopped
	task.UpdatedAt = s.now.Add(time.Duration(len(s.events)+1) * time.Second)
	s.tasks[id] = task
	return task, nil
}

func (s *fakeStore) ListDeploymentsByStatus(_ context.Context, status types.DeploymentStatus) ([]types.Deployment, error) {
	if s.deployment.Status == status {
		return []types.Deployment{s.deployment}, nil
	}
	return nil, nil
}

func (s *fakeStore) UpdateDeploymentStatus(_ context.Context, id types.DeploymentID, status types.DeploymentStatus, expectedUpdatedAt time.Time) (types.Deployment, error) {
	if s.deployment.ID != id {
		return types.Deployment{}, store.ErrNotFound
	}
	if !s.deployment.UpdatedAt.Equal(expectedUpdatedAt) {
		return types.Deployment{}, store.ErrConflict
	}
	s.deployment.Status = status
	s.deployment.UpdatedAt = s.deployment.UpdatedAt.Add(time.Second)
	if status == types.DeploymentRunning && s.deployment.StartedAt.IsZero() {
		s.deployment.StartedAt = s.deployment.UpdatedAt
	}
	if status == types.DeploymentSucceeded || status == types.DeploymentFailed {
		s.deployment.CompletedAt = s.deployment.UpdatedAt
	}
	return s.deployment, nil
}

func (s *fakeStore) AppendEvent(_ context.Context, event types.Event) (types.Event, error) {
	if event.Type == "" {
		return types.Event{}, errors.New("event type is required")
	}
	s.events = append(s.events, event)
	return event, nil
}

func (s *fakeStore) markNewestNewTask(status types.TaskStatus) {
	var selected types.TaskID
	for id, task := range s.tasks {
		if task.Version == 2 && task.ActualStatus != types.TaskHealthy {
			if selected == "" || id > selected {
				selected = id
			}
		}
	}
	task := s.tasks[selected]
	task.ActualStatus = status
	s.tasks[selected] = task
}

func assertTaskVersions(t *testing.T, tasks map[types.TaskID]types.Task, want map[int64]int) {
	t.Helper()
	got := map[int64]int{}
	for _, task := range tasks {
		if isActive(task) {
			got[task.Version]++
		}
	}
	for version, count := range want {
		if got[version] != count {
			t.Fatalf("expected %d active tasks at version %d, got %d (all=%#v)", count, version, got[version], got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected task versions: got %#v want %#v", got, want)
	}
}

func assertTaskCounts(t *testing.T, tasks map[types.TaskID]types.Task, active int, stopped int) {
	t.Helper()
	var gotActive, gotStopped int
	for _, task := range tasks {
		if isActive(task) {
			gotActive++
			continue
		}
		if task.DesiredStatus == types.TaskStopped {
			gotStopped++
		}
	}
	if gotActive != active || gotStopped != stopped {
		t.Fatalf("expected active=%d stopped=%d, got active=%d stopped=%d", active, stopped, gotActive, gotStopped)
	}
}
