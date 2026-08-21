package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/alekpopovic/orch/internal/namespace"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestNamespaceIsolationAndDuplicateNames(t *testing.T) {
	service := NewMemoryService()
	for _, name := range []string{"team-a", "team-b"} {
		if _, err := service.CreateNamespace(context.Background(), name); err != nil {
			t.Fatalf("create namespace %s: %v", name, err)
		}
	}
	spec := types.ServiceSpec{Name: "api", Image: "nginx:1.27", Replicas: 0}
	aCtx := namespace.WithContext(context.Background(), "team-a")
	bCtx := namespace.WithContext(context.Background(), "team-b")
	aService, err := service.CreateService(aCtx, spec)
	if err != nil {
		t.Fatalf("create team-a service: %v", err)
	}
	bService, err := service.CreateService(bCtx, spec)
	if err != nil {
		t.Fatalf("same service name should be allowed in team-b: %v", err)
	}
	if aService.ID == bService.ID || aService.Namespace != "team-a" || bService.Namespace != "team-b" {
		t.Fatalf("unexpected namespace services: %#v %#v", aService, bService)
	}
	items, err := service.ListServices(aCtx)
	if err != nil || len(items) != 1 || items[0].ID != aService.ID {
		t.Fatalf("team-a list leaked namespace: items=%#v err=%v", items, err)
	}
	if _, err := service.GetService(aCtx, bService.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-namespace read should look not found, got %v", err)
	}
}

func TestSecretReferencesCannotCrossNamespace(t *testing.T) {
	service := NewMemoryService()
	for _, name := range []string{"team-a", "team-b"} {
		if _, err := service.CreateNamespace(context.Background(), name); err != nil {
			t.Fatal(err)
		}
	}
	aCtx := namespace.WithContext(context.Background(), "team-a")
	bCtx := namespace.WithContext(context.Background(), "team-b")
	if _, err := service.CreateSecret(aCtx, "database", "secret"); err != nil {
		t.Fatalf("create secret: %v", err)
	}
	_, err := service.CreateService(bCtx, types.ServiceSpec{
		Name: "api", Image: "nginx:1.27", Replicas: 0,
		SecretRefs: []types.SecretRef{{Name: "database", Env: "DATABASE_URL"}},
	})
	if !errors.Is(err, store.ErrInvalidState) {
		t.Fatalf("expected cross-namespace secret reference rejection, got %v", err)
	}
}
