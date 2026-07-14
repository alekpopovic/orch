package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekpopovic/orch/internal/api"
)

func TestAPIErrorRendering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(api.ErrorResponse{
			Error: api.RequestError{
				Code:      "conflict",
				Message:   "rollout already active",
				RequestID: "req-123",
				Details: map[string]any{
					"service": "api",
					"token":   "secret-token",
				},
			},
		})
	}))
	defer server.Close()

	client, err := NewAPIClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.ListNodes(context.Background())
	if err == nil {
		t.Fatalf("expected API error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T %[1]v", err)
	}
	rendered := err.Error()
	for _, want := range []string{"conflict: rollout already active", "service=api", "token=[REDACTED]", "request_id=req-123"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "secret-token") {
		t.Fatalf("rendered error leaked token: %q", rendered)
	}
}
