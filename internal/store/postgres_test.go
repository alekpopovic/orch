package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	env, secretRefs, ports, requirements, healthcheck, restartPolicy, constraints, err := serviceJSON(spec)
	if err != nil {
		t.Fatalf("encode service json: %v", err)
	}

	var got types.ServiceSpec
	got.Name = spec.Name
	got.Image = spec.Image
	got.Replicas = spec.Replicas
	if err := decodeServiceJSON(&got, env, secretRefs, ports, requirements, healthcheck, restartPolicy, constraints); err != nil {
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
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := store.UpdateTaskStatus(ctx, task.ID, types.TaskRunning, types.TaskRunning, "container-1", "", task.UpdatedAt); err != nil {
		t.Fatalf("update task status: %v", err)
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
	events, err := store.ListEventsForObject(ctx, "task", string(task.ID), 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
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
		"000001_initial_schema.down.sql",
		"000001_initial_schema.up.sql",
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

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}
