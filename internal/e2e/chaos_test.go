package e2e

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/rollout"
	"github.com/alekpopovic/orch/internal/scheduler"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestChaosScenarios(t *testing.T) {
	t.Run("agent restarts during task start", func(t *testing.T) {
		ctx := context.Background()
		cp, node := chaosControlPlane(t)
		service := chaosService(t, ctx, cp, "restart-during-start", 1)
		task := chaosTask(t, ctx, cp, service.ID)

		if _, err := cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{TaskID: task.ID, NodeID: node, Status: types.TaskRunning, ContainerID: "container-1"}); err != nil {
			t.Fatalf("first running report: %v", err)
		}
		if _, err := cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{TaskID: task.ID, NodeID: node, Status: types.TaskRunning, ContainerID: "container-1"}); err != nil {
			t.Fatalf("idempotent running report after restart: %v", err)
		}
		tasks, err := cp.ListTasks(ctx, controlplane.TaskFilter{ServiceID: service.ID})
		if err != nil {
			t.Fatalf("list tasks: %v", err)
		}
		if len(tasks) != 1 || tasks[0].ActualStatus != types.TaskRunning {
			t.Fatalf("expected one running task after restart, got %#v", tasks)
		}
	})

	t.Run("server restarts during rollout", func(t *testing.T) {
		ctx := context.Background()
		cp, node := chaosControlPlane(t)
		service := chaosService(t, ctx, cp, "server-restart-rollout", 1)
		task := chaosTask(t, ctx, cp, service.ID)
		_, _ = cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{TaskID: task.ID, NodeID: node, Status: types.TaskHealthy, ContainerID: "old"})
		deployment, err := cp.RolloutService(ctx, service.ID, controlplane.RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1, MaxSurge: 1})
		if err != nil {
			t.Fatalf("start rollout: %v", err)
		}
		controller := rollout.NewController(cp, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err := controller.RunOnce(ctx); err != nil {
			t.Fatalf("rollout controller after restart: %v", err)
		}
		got, err := cp.GetDeployment(ctx, deployment.ID)
		if err != nil {
			t.Fatalf("get deployment: %v", err)
		}
		if got.Status != types.DeploymentRunning && got.Status != types.DeploymentSucceeded {
			t.Fatalf("expected rollout to progress safely, got %q", got.Status)
		}
	})

	t.Run("docker runtime fails to start container", func(t *testing.T) {
		ctx := context.Background()
		cp, node := chaosControlPlane(t)
		service := chaosService(t, ctx, cp, "runtime-start-fails", 1)
		task := chaosTask(t, ctx, cp, service.ID)
		failed, err := cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{TaskID: task.ID, NodeID: node, Status: types.TaskFailed, FailureReason: "docker start failed"})
		if err != nil {
			t.Fatalf("report failed task: %v", err)
		}
		if failed.ActualStatus != types.TaskFailed || failed.FailureReason == "" {
			t.Fatalf("expected failed task with reason, got %#v", failed)
		}
		chaosAssertEvent(t, ctx, cp, events.TypeTaskFailed)
	})

	t.Run("image pull fails repeatedly", func(t *testing.T) {
		ctx := context.Background()
		cp, node := chaosControlPlane(t)
		service := chaosService(t, ctx, cp, "image-pull-fails", 1)
		task := chaosTask(t, ctx, cp, service.ID)
		for attempt := 0; attempt < 2; attempt++ {
			if _, err := cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{TaskID: task.ID, NodeID: node, Status: types.TaskFailed, FailureReason: "image pull failed"}); err != nil {
				t.Fatalf("report image pull failure %d: %v", attempt, err)
			}
		}
		got, err := cp.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if got.ActualStatus != types.TaskFailed || !strings.Contains(got.FailureReason, "image pull") {
			t.Fatalf("expected stable failed image-pull task, got %#v", got)
		}
	})

	t.Run("node goes offline during scale-up", func(t *testing.T) {
		ctx := context.Background()
		cp, node := chaosControlPlane(t)
		service := chaosService(t, ctx, cp, "offline-during-scale", 1)
		if _, err := cp.ScaleService(ctx, service.ID, 4); err != nil {
			t.Fatalf("scale service: %v", err)
		}
		if _, err := cp.HeartbeatNode(ctx, controlplane.NodeHeartbeat{NodeID: node, Shutdown: true}); err != nil {
			t.Fatalf("mark node shutdown: %v", err)
		}
		nodes, err := cp.ListNodes(ctx)
		if err != nil {
			t.Fatalf("list nodes: %v", err)
		}
		if nodes[0].Status != types.NodeOffline {
			t.Fatalf("expected offline node, got %#v", nodes[0])
		}
		chaosAssertEvent(t, ctx, cp, events.TypeNodeShutdown)
	})

	t.Run("node returns with stale containers", func(t *testing.T) {
		ctx := context.Background()
		cp, node := chaosControlPlane(t)
		service := chaosService(t, ctx, cp, "stale-containers", 1)
		task := chaosTask(t, ctx, cp, service.ID)
		if _, err := cp.HeartbeatNode(ctx, controlplane.NodeHeartbeat{NodeID: node, Shutdown: true}); err != nil {
			t.Fatalf("node shutdown: %v", err)
		}
		if _, err := cp.HeartbeatNode(ctx, controlplane.NodeHeartbeat{NodeID: node}); err != nil {
			t.Fatalf("node return: %v", err)
		}
		reported, err := cp.ReportTaskStatus(ctx, controlplane.TaskStatusReport{TaskID: task.ID, NodeID: node, Status: types.TaskRunning, ContainerID: "stale-container"})
		if err != nil {
			t.Fatalf("report stale container after node return: %v", err)
		}
		if reported.ActualStatus != types.TaskRunning {
			t.Fatalf("expected stale container to converge to running task, got %#v", reported)
		}
		chaosAssertEvent(t, ctx, cp, events.TypeNodeHeartbeat)
	})

	t.Run("database write fails during scheduler assignment", func(t *testing.T) {
		errWrite := errors.New("database write failed")
		store := &chaosSchedulerStore{assignErr: errWrite}
		_, err := scheduler.New(store).RunOnce(context.Background())
		if !errors.Is(err, errWrite) {
			t.Fatalf("expected scheduler write failure, got %v", err)
		}
		if len(store.events) != 0 {
			t.Fatalf("expected no assignment event after failed write, got %#v", store.events)
		}
	})

	t.Run("user deletes service during rollout", func(t *testing.T) {
		ctx := context.Background()
		cp, _ := chaosControlPlane(t)
		service := chaosService(t, ctx, cp, "delete-during-rollout", 1)
		if _, err := cp.RolloutService(ctx, service.ID, controlplane.RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1, MaxSurge: 1}); err != nil {
			t.Fatalf("start rollout: %v", err)
		}
		if err := cp.DeleteService(ctx, service.ID); err != nil {
			t.Fatalf("delete service: %v", err)
		}
		got, err := cp.GetService(ctx, service.ID)
		if err != nil {
			t.Fatalf("get service: %v", err)
		}
		if got.Status != types.ServiceDeleting && got.Status != types.ServiceDeleted {
			t.Fatalf("expected deleting/deleted service, got %q", got.Status)
		}
		chaosAssertEvent(t, ctx, cp, events.TypeServiceDeletionStarted)
	})
}

func chaosControlPlane(t *testing.T) (*controlplane.MemoryService, types.NodeID) {
	t.Helper()
	cp := controlplane.NewMemoryService()
	registered, err := cp.RegisterNode(context.Background(), controlplane.NodeRegistration{
		Name:             "node-a",
		AdvertiseAddress: "10.0.0.10",
		Capacity:         types.Resources{CPU: 4000, Memory: 1024 * 1024 * 1024},
		Allocatable:      types.Resources{CPU: 4000, Memory: 1024 * 1024 * 1024},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}
	return cp, registered.Node.ID
}

func chaosService(t *testing.T, ctx context.Context, cp *controlplane.MemoryService, name string, replicas int) types.Service {
	t.Helper()
	service, err := cp.CreateService(ctx, types.ServiceSpec{
		Name:     name,
		Image:    "nginx:1.27",
		Replicas: replicas,
		ResourceRequirements: types.ResourceRequirements{
			Requests: types.Resources{CPU: 100, Memory: 128},
			Limits:   types.Resources{CPU: 100, Memory: 128},
		},
		RestartPolicy: types.RestartPolicy{Condition: types.RestartNever},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return service
}

func chaosTask(t *testing.T, ctx context.Context, cp *controlplane.MemoryService, serviceID types.ServiceID) types.Task {
	t.Helper()
	tasks, err := cp.ListTasks(ctx, controlplane.TaskFilter{ServiceID: serviceID})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("expected at least one task")
	}
	return tasks[0]
}

func chaosAssertEvent(t *testing.T, ctx context.Context, cp *controlplane.MemoryService, eventType string) {
	t.Helper()
	events, err := cp.ListEvents(ctx, events.Filter{Type: eventType})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected event %s", eventType)
	}
}

type chaosSchedulerStore struct {
	assignErr error
	events    []types.Event
}

func (s *chaosSchedulerStore) ListTasksByStatus(context.Context, types.TaskStatus) ([]types.Task, error) {
	return []types.Task{{
		ID:            "00000000-0000-4000-8000-000000000101",
		ServiceID:     "00000000-0000-4000-8000-000000000102",
		DesiredStatus: types.TaskRunning,
		ActualStatus:  types.TaskPending,
		Image:         "nginx:1.27",
		Version:       1,
		CreatedAt:     time.Unix(1, 0).UTC(),
		UpdatedAt:     time.Unix(1, 0).UTC(),
	}}, nil
}

func (s *chaosSchedulerStore) ListNodesByStatus(context.Context, types.NodeStatus) ([]types.Node, error) {
	return []types.Node{{
		ID:          "00000000-0000-4000-8000-000000000103",
		Status:      types.NodeReady,
		Capacity:    types.Resources{CPU: 1000, Memory: 1024},
		Allocatable: types.Resources{CPU: 1000, Memory: 1024},
	}}, nil
}

func (s *chaosSchedulerStore) ListTasksByNode(context.Context, types.NodeID) ([]types.Task, error) {
	return nil, nil
}

func (s *chaosSchedulerStore) GetService(context.Context, types.ServiceID) (types.Service, error) {
	return types.Service{
		ID: "00000000-0000-4000-8000-000000000102",
		Spec: types.ServiceSpec{
			Name:                 "db-fail",
			Image:                "nginx:1.27",
			Replicas:             1,
			ResourceRequirements: types.ResourceRequirements{Requests: types.Resources{CPU: 100, Memory: 128}},
		},
	}, nil
}

func (s *chaosSchedulerStore) AssignTask(context.Context, types.TaskID, types.NodeID, []types.Port, time.Time) (types.Task, error) {
	return types.Task{}, s.assignErr
}

func (s *chaosSchedulerStore) AppendEvent(_ context.Context, event types.Event) (types.Event, error) {
	s.events = append(s.events, event)
	return event, nil
}
