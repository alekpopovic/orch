package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestRolloutWhileRolloutInProgress(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	created := createOperationLockTestService(t, ctx, service)

	first, err := service.RolloutService(ctx, created.ID, RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1, MaxSurge: 1})
	if err != nil {
		t.Fatalf("first rollout: %v", err)
	}
	repeated, err := service.RolloutService(ctx, created.ID, RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1, MaxSurge: 1})
	if err != nil {
		t.Fatalf("repeated rollout should be idempotent: %v", err)
	}
	if repeated.ID != first.ID {
		t.Fatalf("expected repeated rollout to return %s, got %s", first.ID, repeated.ID)
	}
	if _, err := service.RolloutService(ctx, created.ID, RolloutSpec{Image: "nginx:1.29", MaxUnavailable: 1, MaxSurge: 1}); !isOperationConflict(err, "rollout") {
		t.Fatalf("expected rollout conflict, got %v", err)
	}
}

func TestRollbackDuringActiveRolloutConflicts(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	created := createOperationLockTestService(t, ctx, service)
	if _, err := service.RolloutService(ctx, created.ID, RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1, MaxSurge: 1}); err != nil {
		t.Fatalf("rollout: %v", err)
	}

	if _, err := service.RollbackService(ctx, created.ID); !isOperationConflict(err, "rollout") {
		t.Fatalf("expected rollback conflict, got %v", err)
	}
}

func TestDeleteWinsOverActiveRollout(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	created := createOperationLockTestService(t, ctx, service)
	deployment, err := service.RolloutService(ctx, created.ID, RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1, MaxSurge: 1})
	if err != nil {
		t.Fatalf("rollout: %v", err)
	}

	if err := service.DeleteService(ctx, created.ID); err != nil {
		t.Fatalf("delete should win: %v", err)
	}
	updatedDeployment, err := service.GetDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if updatedDeployment.Status != types.DeploymentFailed {
		t.Fatalf("expected active deployment failed after delete, got %q", updatedDeployment.Status)
	}
	if _, err := service.RolloutService(ctx, created.ID, RolloutSpec{Image: "nginx:1.29", MaxUnavailable: 1, MaxSurge: 1}); !isOperationConflict(err, "delete") {
		t.Fatalf("expected rollout/delete conflict, got %v", err)
	}
}

func TestScaleAndDeleteOperationOrdering(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	created := createOperationLockTestService(t, ctx, service)

	if err := service.DeleteService(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := service.ScaleService(ctx, created.ID, 3); !isOperationConflict(err, "delete") {
		t.Fatalf("expected scale/delete conflict, got %v", err)
	}

	other := createOperationLockTestServiceNamed(t, ctx, service, "worker")
	if _, err := service.ScaleService(ctx, other.ID, 3); err != nil {
		t.Fatalf("scale before delete: %v", err)
	}
	if err := service.DeleteService(ctx, other.ID); err != nil {
		t.Fatalf("delete should win after scale: %v", err)
	}
}

func TestRepeatedDeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	created := createOperationLockTestService(t, ctx, service)

	if err := service.DeleteService(ctx, created.ID); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := service.DeleteService(ctx, created.ID); err != nil {
		t.Fatalf("second delete should be idempotent: %v", err)
	}
}

func createOperationLockTestService(t *testing.T, ctx context.Context, service *MemoryService) types.Service {
	t.Helper()
	return createOperationLockTestServiceNamed(t, ctx, service, "api")
}

func createOperationLockTestServiceNamed(t *testing.T, ctx context.Context, service *MemoryService, name string) types.Service {
	t.Helper()
	created, err := service.CreateService(ctx, types.ServiceSpec{
		Name:                 name,
		Image:                "nginx:1.27",
		Replicas:             2,
		ResourceRequirements: types.ResourceRequirements{},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return created
}

func isOperationConflict(err error, operation string) bool {
	return errors.Is(err, store.ErrConflict) && strings.Contains(err.Error(), "operation already in progress: "+operation)
}
