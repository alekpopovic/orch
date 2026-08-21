package controlplane

import (
	"context"
	"testing"

	"github.com/alekpopovic/orch/internal/policy"
	"github.com/alekpopovic/orch/pkg/types"
)

type fixedImageResolver struct {
	digest string
}

func (resolver fixedImageResolver) Resolve(_ context.Context, requested string) (types.ImageMetadata, error) {
	return types.ImageMetadata{RequestedImage: requested, Registry: "ghcr.io", Name: "acme/api", Tag: "v2", Digest: resolver.digest}, nil
}

func TestRolloutStoresDigestAndTasksUsePinnedImage(t *testing.T) {
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	service := NewMemoryService(
		WithClusterPolicy(policy.ClusterPolicy{RequireDigest: true}),
		WithImageResolver(fixedImageResolver{digest: digest}),
	)
	ctx := context.Background()
	if _, err := service.RegisterNode(ctx, NodeRegistration{Name: "node", AdvertiseAddress: "http://node", Capacity: types.Resources{CPU: 1000, Memory: 1 << 30}, Allocatable: types.Resources{CPU: 1000, Memory: 1 << 30}}); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateService(ctx, quotaServiceSpec("api", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RolloutService(ctx, created.ID, RolloutSpec{Image: "ghcr.io/acme/api:v2", MaxUnavailable: 1, MaxSurge: 1}); err != nil {
		t.Fatal(err)
	}
	updated, err := service.GetService(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Spec.ImageMetadata.Digest != digest || !updated.Spec.ImageMetadata.Pinned {
		t.Fatalf("rollout did not persist pinned digest: %#v", updated.Spec.ImageMetadata)
	}
	// Complete the rollout task creation through the rollout controller's shared state helper.
	service.mu.Lock()
	for id, deployment := range service.deployments {
		deployment.Status = types.DeploymentSucceeded
		service.deployments[id] = deployment
	}
	service.reconcileServiceTasksLocked(updated, service.now())
	service.mu.Unlock()
	tasks, err := service.ListTasks(ctx, TaskFilter{ServiceID: created.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := "ghcr.io/acme/api@" + digest
	found := false
	for _, task := range tasks {
		if task.Version == updated.DeploymentVersion && task.Image == want && task.ResolvedImageDigest == digest {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected digest-pinned task image %q, got %#v", want, tasks)
	}
}
