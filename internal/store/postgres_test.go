package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapPostgresError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "duplicate",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "services_name_key"},
			want: ErrDuplicate,
		},
		{
			name: "foreign key invalid state",
			err:  &pgconn.PgError{Code: "23503", Message: "violates foreign key constraint"},
			want: ErrInvalidState,
		},
		{
			name: "check invalid state",
			err:  &pgconn.PgError{Code: "23514", Message: "violates check constraint"},
			want: ErrInvalidState,
		},
		{
			name: "serialization conflict",
			err:  &pgconn.PgError{Code: "40001", Message: "could not serialize access"},
			want: ErrConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapPostgresError(tt.err); !errors.Is(got, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestServiceJSONRoundTrip(t *testing.T) {
	spec := serviceSpecFixture()
	env, secretRefs, ports, requirements, healthcheck, restartPolicy, constraints, routes, err := serviceJSON(spec)
	if err != nil {
		t.Fatalf("encode service json: %v", err)
	}

	var got types.ServiceSpec
	got.Name = spec.Name
	got.Image = spec.Image
	got.Replicas = spec.Replicas
	if err := decodeServiceJSON(&got, env, secretRefs, ports, requirements, healthcheck, restartPolicy, constraints, routes); err != nil {
		t.Fatalf("decode service json: %v", err)
	}

	if got.Env["APP_ENV"] != "test" {
		t.Fatalf("expected env to round trip, got %#v", got.Env)
	}
	if got.Healthcheck == nil || got.Healthcheck.Path != "/healthz" {
		t.Fatalf("expected healthcheck to round trip, got %#v", got.Healthcheck)
	}
	if len(got.Ports) != 1 || got.Ports[0].ContainerPort != 8080 {
		t.Fatalf("expected ports to round trip, got %#v", got.Ports)
	}
	if len(got.Routes) != 1 || got.Routes[0].Host != "api.example.com" {
		t.Fatalf("expected routes to round trip, got %#v", got.Routes)
	}
}

func TestPostgresStoreIntegration(t *testing.T) {
	databaseURL := os.Getenv("ORCH_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ORCH_INTEGRATION_DATABASE_URL to run PostgreSQL integration tests")
	}

	ctx := context.Background()
	pool, err := ConnectPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	migrate(t, ctx, pool)
	store := NewPostgresStore(pool)

	node, err := store.CreateNode(ctx, types.NodeSpec{
		Hostname:         "node-1",
		AdvertiseAddress: "10.0.0.10",
		Labels:           map[string]string{"region": "test"},
		Capacity:         types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
		Allocatable:      types.Resources{CPU: 3500, Memory: 7 * 1024 * 1024 * 1024},
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	node.Status = types.NodeReady
	updatedNode, err := store.UpdateNode(ctx, node, node.UpdatedAt)
	if err != nil {
		t.Fatalf("update node: %v", err)
	}
	if _, err := store.UpdateNode(ctx, updatedNode, node.UpdatedAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale node update to conflict, got %v", err)
	}

	service, err := store.CreateService(ctx, serviceSpecFixture())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if _, err := store.CreateService(ctx, serviceSpecFixture()); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected duplicate service name, got %v", err)
	}

	task, err := store.CreateTask(ctx, types.Task{
		ServiceID:     service.ID,
		NodeID:        updatedNode.ID,
		DesiredStatus: types.TaskRunning,
		ActualStatus:  types.TaskPending,
		Image:         service.Spec.Image,
		Version:       service.DeploymentVersion,
		Ports:         []types.Port{{Protocol: types.PortTCP, ContainerPort: 8080}},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	assignedPorts := []types.Port{{Protocol: types.PortTCP, ContainerPort: 8080, PublishedPort: 18080}}
	assigned, err := store.AssignTask(ctx, task.ID, updatedNode.ID, assignedPorts, task.UpdatedAt)
	if err != nil {
		t.Fatalf("assign task: %v", err)
	}
	if !portsEqual(assigned.Ports, assignedPorts) {
		t.Fatalf("expected assigned ports %#v, got %#v", assignedPorts, assigned.Ports)
	}
	if _, err := store.AssignTask(ctx, task.ID, updatedNode.ID, assignedPorts, task.UpdatedAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale duplicate assignment to conflict, got %v", err)
	}
	running, err := store.UpdateTaskStatus(ctx, task.ID, types.TaskRunning, types.TaskRunning, "container-1", "", assigned.UpdatedAt)
	if err != nil {
		t.Fatalf("update task status: %v", err)
	}
	stopped, err := store.StopTask(ctx, task.ID, running.UpdatedAt)
	if err != nil {
		t.Fatalf("stop task: %v", err)
	}
	stoppedAgain, err := store.StopTask(ctx, task.ID, running.UpdatedAt)
	if err != nil {
		t.Fatalf("idempotent stop task: %v", err)
	}
	if stoppedAgain.DesiredStatus != types.TaskStopped || stoppedAgain.ID != stopped.ID {
		t.Fatalf("expected idempotent stop to return stopped task, got %#v", stoppedAgain)
	}

	deployment, err := store.CreateDeployment(ctx, types.Deployment{
		ServiceID:      service.ID,
		FromVersion:    1,
		ToVersion:      2,
		Strategy:       types.RolloutRollingUpdate,
		Status:         types.DeploymentPending,
		MaxUnavailable: 1,
		MaxSurge:       1,
	})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if _, err := store.UpdateDeploymentStatus(ctx, deployment.ID, types.DeploymentRunning, deployment.UpdatedAt); err != nil {
		t.Fatalf("update deployment: %v", err)
	}

	if _, err := store.AppendEvent(ctx, types.Event{
		Type:              "task.started",
		Severity:          types.EventInfo,
		Source:            "integration-test",
		Message:           "task started",
		RelatedObjectType: "task",
		RelatedObjectID:   string(task.ID),
		Timestamp:         time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	listedEvents, err := store.ListEvents(ctx, events.Filter{TaskID: task.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(listedEvents) != 1 {
		t.Fatalf("expected one event, got %d", len(listedEvents))
	}
}

func TestPostgresStoreWithTxRollsBackOnError(t *testing.T) {
	databaseURL := os.Getenv("ORCH_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ORCH_INTEGRATION_DATABASE_URL to run PostgreSQL integration tests")
	}

	ctx := context.Background()
	pool, err := ConnectPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	migrate(t, ctx, pool)
	pgStore := NewPostgresStore(pool)
	var durable Store = pgStore
	errRollback := errors.New("rollback tx")
	var serviceID types.ServiceID

	err = WithTx(ctx, durable, func(txCtx context.Context, tx Store) error {
		service, err := tx.CreateService(txCtx, serviceSpecFixture())
		if err != nil {
			return err
		}
		serviceID = service.ID
		if _, err := tx.AppendEvent(txCtx, types.Event{
			Type:              events.TypeServiceCreated,
			Severity:          types.EventInfo,
			Source:            "test",
			Message:           "service created",
			RelatedObjectType: "service",
			RelatedObjectID:   string(service.ID),
		}); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	if _, err := pgStore.GetService(ctx, serviceID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected service insert to roll back, got %v", err)
	}
	events, err := pgStore.ListEvents(ctx, events.Filter{ServiceID: serviceID})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected event insert to roll back, got %#v", events)
	}
}

func TestPostgresStoreConcurrentAssignTaskOnce(t *testing.T) {
	databaseURL := os.Getenv("ORCH_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ORCH_INTEGRATION_DATABASE_URL to run PostgreSQL integration tests")
	}

	ctx := context.Background()
	pool, err := ConnectPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	migrate(t, ctx, pool)
	pgStore := NewPostgresStore(pool)
	var durable Store = pgStore
	node, err := pgStore.CreateNode(ctx, types.NodeSpec{
		Hostname:         "node-1",
		AdvertiseAddress: "10.0.0.10",
		Capacity:         types.Resources{CPU: 4000, Memory: 1024},
		Allocatable:      types.Resources{CPU: 3000, Memory: 512},
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	service, err := pgStore.CreateService(ctx, serviceSpecFixture())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	task, err := pgStore.CreateTask(ctx, types.Task{
		ServiceID:     service.ID,
		DesiredStatus: types.TaskRunning,
		ActualStatus:  types.TaskPending,
		Image:         service.Spec.Image,
		Version:       service.DeploymentVersion,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- WithTx(ctx, durable, func(txCtx context.Context, tx Store) error {
				assigned, err := tx.AssignTask(txCtx, task.ID, node.ID, nil, task.UpdatedAt)
				if err != nil {
					return err
				}
				_, err = tx.AppendEvent(txCtx, types.Event{
					Type:              events.TypeTaskAssigned,
					Severity:          types.EventInfo,
					Source:            "test",
					Message:           "task assigned",
					RelatedObjectType: "task",
					RelatedObjectID:   string(assigned.ID),
				})
				return err
			})
		}()
	}

	successes := 0
	conflicts := 0
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected assignment error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one conflict, got successes=%d conflicts=%d", successes, conflicts)
	}
	assigned, err := pgStore.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get assigned task: %v", err)
	}
	if assigned.ActualStatus != types.TaskAssigned || assigned.NodeID != node.ID {
		t.Fatalf("expected task assigned once, got %#v", assigned)
	}
	events, err := pgStore.ListEvents(ctx, events.Filter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("list task events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one assignment event, got %#v", events)
	}
}

func serviceSpecFixture() types.ServiceSpec {
	return types.ServiceSpec{
		Name:     "web",
		Image:    "nginx:1.27",
		Replicas: 2,
		Env: map[string]string{
			"APP_ENV": "test",
		},
		SecretRefs: []types.SecretRef{
			{Name: "registry", Key: "password"},
		},
		Ports: []types.Port{
			{Protocol: types.PortTCP, ContainerPort: 8080, PublishedPort: 18080},
		},
		Routes: []types.Route{
			{Host: "api.example.com", PathPrefix: "/", Port: 8080, TLS: true},
		},
		ResourceRequirements: types.ResourceRequirements{
			Requests: types.Resources{CPU: 100, Memory: 128 * 1024 * 1024},
			Limits:   types.Resources{CPU: 500, Memory: 512 * 1024 * 1024},
		},
		Healthcheck: &types.Healthcheck{
			Type:               types.HealthcheckHTTP,
			Path:               "/healthz",
			Port:               8080,
			Interval:           10 * time.Second,
			Timeout:            2 * time.Second,
			HealthyThreshold:   1,
			UnhealthyThreshold: 3,
		},
		RestartPolicy: types.RestartPolicy{Condition: types.RestartOnFailure, MaxAttempts: 3},
		PlacementConstraints: []types.PlacementConstraint{
			{Key: "region", Operator: types.ConstraintEquals, Value: "test"},
		},
	}
}

func migrate(t *testing.T, ctx context.Context, pool execer) {
	t.Helper()

	for _, file := range []string{
		"000003_service_routes.down.sql",
		"000002_task_ports.down.sql",
		"000001_initial_schema.down.sql",
		"000001_initial_schema.up.sql",
		"000002_task_ports.up.sql",
		"000003_service_routes.up.sql",
	} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", file))
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", file, err)
		}
	}
}

func portsEqual(left, right []types.Port) bool {
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

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}
