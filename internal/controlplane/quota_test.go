package controlplane

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/alekpopovic/orch/internal/quota"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestResourceQuotaAllowsAndDeniesUsage(t *testing.T) {
	service := NewMemoryService()
	ctx := context.Background()
	if _, _, err := service.SetResourceQuota(ctx, types.ResourceQuota{MaxServices: 1, MaxReplicas: 2, MaxSecrets: 1, MaxRegistryCredentials: 1}); err != nil {
		t.Fatalf("set quota: %v", err)
	}
	created, err := service.CreateService(ctx, quotaServiceSpec("api", 2))
	if err != nil {
		t.Fatalf("quota should allow service: %v", err)
	}
	if _, err := service.CreateService(ctx, quotaServiceSpec("worker", 0)); err == nil {
		t.Fatal("expected max services quota rejection")
	} else {
		var quotaErr *quota.Error
		if !errors.As(err, &quotaErr) || quotaErr.Resource != "services" {
			t.Fatalf("expected services quota error, got %v", err)
		}
	}
	if err := service.DeleteService(ctx, created.ID); err != nil {
		t.Fatalf("delete service: %v", err)
	}
	if _, err := service.CreateService(ctx, quotaServiceSpec("worker", 1)); err != nil {
		t.Fatalf("deleted service should release quota: %v", err)
	}
}

func TestConcurrentScaleCannotExceedQuota(t *testing.T) {
	service := NewMemoryService()
	ctx := context.Background()
	if _, _, err := service.SetResourceQuota(ctx, types.ResourceQuota{MaxReplicas: 3}); err != nil {
		t.Fatal(err)
	}
	first, err := service.CreateService(ctx, quotaServiceSpec("first", 1))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateService(ctx, quotaServiceSpec("second", 1))
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []types.ServiceID{first.ID, second.ID} {
		wg.Add(1)
		go func(id types.ServiceID) {
			defer wg.Done()
			<-start
			_, err := service.ScaleService(ctx, id, 2)
			results <- err
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded, denied := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		var quotaErr *quota.Error
		if errors.As(err, &quotaErr) {
			denied++
			continue
		}
		t.Fatalf("unexpected scale error: %v", err)
	}
	if succeeded != 1 || denied != 1 {
		t.Fatalf("expected one success and one quota denial, got success=%d denied=%d", succeeded, denied)
	}
}

func TestSecretAndRegistryCredentialQuotas(t *testing.T) {
	service := NewMemoryService()
	ctx := context.Background()
	if _, _, err := service.SetResourceQuota(ctx, types.ResourceQuota{MaxSecrets: 1, MaxRegistryCredentials: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateSecret(ctx, "one", "value"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateSecret(ctx, "two", "value"); err == nil {
		t.Fatal("expected secret quota rejection")
	}
	credential := RegistryCredentialSpec{ID: "one", Registry: "ghcr.io", Username: "robot", Password: "secret"}
	if _, err := service.CreateRegistryCredential(ctx, credential); err != nil {
		t.Fatal(err)
	}
	credential.ID = "two"
	if _, err := service.CreateRegistryCredential(ctx, credential); err == nil {
		t.Fatal("expected registry credential quota rejection")
	}
}

func quotaServiceSpec(name string, replicas int) types.ServiceSpec {
	return types.ServiceSpec{
		Name: name, Image: "nginx:1.27", Replicas: replicas,
		ResourceRequirements: types.ResourceRequirements{
			Requests: types.Resources{CPU: 100, Memory: 1024},
			Limits:   types.Resources{CPU: 100, Memory: 1024},
		},
	}
}
