package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/auth"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/namespace"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestNamespaceScopedViewerCannotReadAnotherNamespace(t *testing.T) {
	controlPlane := controlplane.NewMemoryService()
	for _, name := range []string{"team-a", "team-b"} {
		if _, err := controlPlane.CreateNamespace(context.Background(), name); err != nil {
			t.Fatal(err)
		}
		ctx := namespace.WithContext(context.Background(), name)
		if _, err := controlPlane.CreateService(ctx, types.ServiceSpec{Name: "api", Image: "nginx:1.27", Replicas: 0}); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewHandler(slog.Default(), controlPlane, WithUserJWT("test-secret"))
	token, err := auth.SignJWT(auth.Claims{
		Subject: "viewer-a", NamespaceRoles: map[string]auth.Role{"team-a": auth.RoleViewer},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, "test-secret")
	if err != nil {
		t.Fatal(err)
	}

	allowed := requestWithHeaders(t, handler, http.MethodGet, "/v1/services", nil, map[string]string{
		"Authorization": "Bearer " + token, "X-Orch-Namespace": "team-a",
	})
	if allowed.Code != http.StatusOK {
		t.Fatalf("expected team-a access, got %d: %s", allowed.Code, allowed.Body.String())
	}
	denied := requestWithHeaders(t, handler, http.MethodGet, "/v1/services", nil, map[string]string{
		"Authorization": "Bearer " + token, "X-Orch-Namespace": "team-b",
	})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected team-b forbidden, got %d: %s", denied.Code, denied.Body.String())
	}
}

func TestNamespaceCRUDAPI(t *testing.T) {
	handler := newTestHandler()
	created := doRequest(t, handler, http.MethodPost, "/v1/namespaces", `{"name":"payments"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create namespace: %d %s", created.Code, created.Body.String())
	}
	listed := doRequest(t, handler, http.MethodGet, "/v1/namespaces", nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"name":"payments"`) {
		t.Fatalf("list namespaces: %d %s", listed.Code, listed.Body.String())
	}
	deleted := doRequest(t, handler, http.MethodDelete, "/v1/namespaces/payments", nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete namespace: %d %s", deleted.Code, deleted.Body.String())
	}
}
