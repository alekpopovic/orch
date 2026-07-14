package client_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/pkg/client"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestClientHealthAndServices(t *testing.T) {
	server := httptest.NewServer(api.NewHandler(slog.Default(), controlplane.NewMemoryService()))
	defer server.Close()

	apiClient, err := client.New(server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	ctx := context.Background()

	health, err := apiClient.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if health.Status != "ok" || health.Time.IsZero() {
		t.Fatalf("unexpected health response %#v", health)
	}

	service, err := apiClient.CreateService(ctx, types.ServiceSpec{
		Name:     "api",
		Image:    "nginx:1.27",
		Replicas: 1,
		ResourceRequirements: types.ResourceRequirements{
			Requests: types.Resources{},
			Limits:   types.Resources{},
		},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if service.ID == "" || service.Spec.Name != "api" {
		t.Fatalf("unexpected service %#v", service)
	}

	services, err := apiClient.ListServices(ctx)
	if err != nil {
		t.Fatalf("list services: %v", err)
	}
	if len(services) != 1 || services[0].ID != service.ID {
		t.Fatalf("unexpected services %#v", services)
	}
}

func TestClientStructuredError(t *testing.T) {
	server := httptest.NewServer(api.NewHandler(slog.Default(), controlplane.NewMemoryService()))
	defer server.Close()

	apiClient, err := client.New(server.URL)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = apiClient.GetService(context.Background(), "not-a-uuid")
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiError *client.APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiError.StatusCode != http.StatusBadRequest || apiError.Code != "invalid_argument" || apiError.RequestID == "" {
		t.Fatalf("unexpected API error %#v", apiError)
	}
}

func TestNormalizeServerURL(t *testing.T) {
	normalized, err := client.NormalizeServerURL(" http://localhost:8080/ ")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if normalized != "http://localhost:8080" {
		t.Fatalf("unexpected normalized URL %q", normalized)
	}
	if _, err := client.NormalizeServerURL("ftp://localhost"); err == nil {
		t.Fatalf("expected invalid scheme")
	}
}
