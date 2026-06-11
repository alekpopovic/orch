package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestRollbackAfterFailedRollout(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	created := createRollbackTestService(t, ctx, service)

	failed, err := service.RolloutService(ctx, created.ID, RolloutSpec{
		Image:          "ghcr.io/example/api:2.0.0",
		MaxUnavailable: 1,
		MaxSurge:       1,
	})
	if err != nil {
		t.Fatalf("start rollout: %v", err)
	}
	if _, err := service.UpdateDeploymentStatus(ctx, failed.ID, types.DeploymentFailed, failed.UpdatedAt); err != nil {
		t.Fatalf("mark rollout failed: %v", err)
	}

	rollback, err := service.RollbackService(ctx, created.ID)
	if err != nil {
		t.Fatalf("rollback service: %v", err)
	}
	if rollback.Status != types.DeploymentRollingBack {
		t.Fatalf("expected rolling_back deployment, got %q", rollback.Status)
	}
	if rollback.FromVersion != 2 || rollback.ToVersion != 1 {
		t.Fatalf("expected rollback from v2 to v1, got from=%d to=%d", rollback.FromVersion, rollback.ToVersion)
	}
	updated, err := service.GetService(ctx, created.ID)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if updated.DeploymentVersion != 1 || updated.Spec.Image != "ghcr.io/example/api:1.0.0" {
		t.Fatalf("expected service restored to v1 image, got version=%d image=%q", updated.DeploymentVersion, updated.Spec.Image)
	}
	history, err := service.GetDeployment(ctx, failed.ID)
	if err != nil {
		t.Fatalf("get failed rollout history: %v", err)
	}
	if history.Status != types.DeploymentFailed {
		t.Fatalf("expected failed rollout history to be preserved, got %q", history.Status)
	}
}

func TestRollbackWhenNoPreviousVersionExists(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	created := createRollbackTestService(t, ctx, service)

	if _, err := service.RollbackService(ctx, created.ID); !errors.Is(err, store.ErrInvalidState) {
		t.Fatalf("expected invalid state, got %v", err)
	}
}

func TestRollbackIsIdempotentWhileInFlight(t *testing.T) {
	ctx := context.Background()
	service := NewMemoryService()
	created := createRollbackTestService(t, ctx, service)

	failed, err := service.RolloutService(ctx, created.ID, RolloutSpec{
		Image:          "ghcr.io/example/api:2.0.0",
		MaxUnavailable: 1,
		MaxSurge:       1,
	})
	if err != nil {
		t.Fatalf("start rollout: %v", err)
	}
	if _, err := service.UpdateDeploymentStatus(ctx, failed.ID, types.DeploymentFailed, failed.UpdatedAt); err != nil {
		t.Fatalf("mark rollout failed: %v", err)
	}

	first, err := service.RollbackService(ctx, created.ID)
	if err != nil {
		t.Fatalf("first rollback: %v", err)
	}
	second, err := service.RollbackService(ctx, created.ID)
	if err != nil {
		t.Fatalf("second rollback: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected idempotent rollback to return %q, got %q", first.ID, second.ID)
	}
}

func createRollbackTestService(t *testing.T, ctx context.Context, service *MemoryService) types.Service {
	t.Helper()
	created, err := service.CreateService(ctx, types.ServiceSpec{
		Name:                 "api",
		Image:                "ghcr.io/example/api:1.0.0",
		Replicas:             2,
		ResourceRequirements: types.ResourceRequirements{},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return created
}
