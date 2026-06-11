package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekpopovic/orch/internal/controlplane"
)

func TestHealthz(t *testing.T) {
	rec := doRequest(t, newTestHandler(), http.MethodGet, "/healthz", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected request id header")
	}
}

func TestListNodes(t *testing.T) {
	rec := doRequest(t, newTestHandler(), http.MethodGet, "/v1/nodes", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var body ListNodesResponse
	decodeResponse(t, rec, &body)
	if len(body.Nodes) != 1 {
		t.Fatalf("expected seeded node, got %d", len(body.Nodes))
	}
}

func TestDrainNode(t *testing.T) {
	handler := newTestHandler()
	rec := doRequest(t, handler, http.MethodPost, "/v1/nodes/00000000-0000-4000-8000-000000000001/drain", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var body NodeResponse
	decodeResponse(t, rec, &body)
	if body.Node.Status != "draining" {
		t.Fatalf("expected node to be draining, got %q", body.Node.Status)
	}
}

func TestCreateService(t *testing.T) {
	rec := doRequest(t, newTestHandler(), http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "web",
			"image": "nginx:1.27",
			"replicas": 2,
			"ports": [{"protocol": "tcp", "container_port": 8080}],
			"resource_requirements": {
				"requests": {"cpu": 100, "memory": 134217728},
				"limits": {"cpu": 500, "memory": 536870912}
			},
			"restart_policy": {"condition": "on_failure", "max_attempts": 3}
		}
	}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var body ServiceResponse
	decodeResponse(t, rec, &body)
	if body.Service.ID == "" {
		t.Fatalf("expected service id")
	}
	if body.Service.Spec.Name != "web" {
		t.Fatalf("expected service name web, got %q", body.Service.Spec.Name)
	}
}

func TestCreateServiceValidationError(t *testing.T) {
	rec := doRequest(t, newTestHandler(), http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "",
			"image": "nginx:1.27",
			"replicas": 1,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var body ErrorResponse
	decodeResponse(t, rec, &body)
	if body.Error.Code != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", body.Error.Code)
	}
	if body.Error.RequestID == "" {
		t.Fatalf("expected request id in error")
	}
}

func TestGetUnknownService(t *testing.T) {
	rec := doRequest(t, newTestHandler(), http.MethodGet, "/v1/services/00000000-0000-4000-8000-000000000999", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	var body ErrorResponse
	decodeResponse(t, rec, &body)
	if body.Error.Code != "not_found" {
		t.Fatalf("expected not_found, got %q", body.Error.Code)
	}
}

func TestInvalidPathID(t *testing.T) {
	rec := doRequest(t, newTestHandler(), http.MethodGet, "/v1/services/not-a-uuid", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestScaleService(t *testing.T) {
	handler := newTestHandler()
	create := doRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "worker",
			"image": "busybox:1.36",
			"replicas": 1,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`)
	var created ServiceResponse
	decodeResponse(t, create, &created)

	rec := doRequest(t, handler, http.MethodPost, "/v1/services/"+string(created.Service.ID)+"/scale", `{"replicas": 4}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var scaled ServiceResponse
	decodeResponse(t, rec, &scaled)
	if scaled.Service.Spec.Replicas != 4 {
		t.Fatalf("expected replicas 4, got %d", scaled.Service.Spec.Replicas)
	}
}

func TestListEvents(t *testing.T) {
	handler := newTestHandler()
	_ = doRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "events",
			"image": "nginx:1.27",
			"replicas": 1,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`)

	rec := doRequest(t, handler, http.MethodGet, "/v1/events?limit=10", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var body ListEventsResponse
	decodeResponse(t, rec, &body)
	if len(body.Events) == 0 {
		t.Fatalf("expected at least one event")
	}
}

func newTestHandler() http.Handler {
	return NewHandler(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), controlplane.NewMemoryService())
}

func doRequest(t *testing.T, handler http.Handler, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	switch value := body.(type) {
	case nil:
		reader = strings.NewReader("")
	case string:
		reader = strings.NewReader(value)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(data))
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-request-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}
