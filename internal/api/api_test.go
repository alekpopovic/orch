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
	"github.com/alekpopovic/orch/pkg/types"
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
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"))
	registered := registerTestNode(t, handler)

	rec := doRequest(t, handler, http.MethodGet, "/v1/nodes", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var body ListNodesResponse
	decodeResponse(t, rec, &body)
	if len(body.Nodes) != 1 {
		t.Fatalf("expected registered node, got %d", len(body.Nodes))
	}
	if body.Nodes[0].ID != registered.Node.ID {
		t.Fatalf("expected node %q, got %q", registered.Node.ID, body.Nodes[0].ID)
	}
}

func TestAgentRegisterRequiresBootstrapToken(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"))
	rec := doRequest(t, handler, http.MethodPost, "/v1/agent/register", `{
		"node_name": "node-a",
		"advertise_address": "10.0.0.10",
		"capacity": {"cpu": 4000, "memory": 1024},
		"allocatable": {"cpu": 4000, "memory": 1024}
	}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestAgentRegisterAndHeartbeat(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"))
	rec := doAgentRequest(t, handler, http.MethodPost, "/v1/agent/register", `{
		"node_name": "node-a",
		"advertise_address": "10.0.0.10",
		"labels": {"role": "app"},
		"capacity": {"cpu": 4000, "memory": 1024},
		"allocatable": {"cpu": 3000, "memory": 512}
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var registered AgentResponse
	decodeResponse(t, rec, &registered)
	if registered.Node.ID == "" || registered.Status != "ready" {
		t.Fatalf("unexpected register response %#v", registered)
	}

	heartbeat := doAgentRequest(t, handler, http.MethodPost, "/v1/agent/heartbeat", `{
		"node_id": "`+string(registered.Node.ID)+`",
		"capacity": {"cpu": 4000, "memory": 1024},
		"allocatable": {"cpu": 3000, "memory": 512}
	}`)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, heartbeat.Code, heartbeat.Body.String())
	}
	var acknowledged AgentResponse
	decodeResponse(t, heartbeat, &acknowledged)
	if acknowledged.Node.LastHeartbeatAt.IsZero() {
		t.Fatalf("expected heartbeat timestamp")
	}
}

func TestDrainNode(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"))
	registered := registerTestNode(t, handler)
	rec := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+string(registered.Node.ID)+"/drain", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var body NodeResponse
	decodeResponse(t, rec, &body)
	if body.Node.Status != "draining" {
		t.Fatalf("expected node to be draining, got %q", body.Node.Status)
	}
}

func TestAgentTaskAssignmentAndStatus(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"))
	registered := registerTestNode(t, handler)

	create := doRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "agent-work",
			"image": "nginx:1.27",
			"replicas": 1,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, create.Code, create.Body.String())
	}

	list := doAgentRequest(t, handler, http.MethodGet, "/v1/agent/tasks?node_id="+string(registered.Node.ID), nil)
	if list.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, list.Code, list.Body.String())
	}
	var tasks AgentTasksResponse
	decodeResponse(t, list, &tasks)
	if len(tasks.Tasks) != 1 {
		t.Fatalf("expected one assigned task, got %d", len(tasks.Tasks))
	}

	report := doAgentRequest(t, handler, http.MethodPost, "/v1/agent/tasks/"+string(tasks.Tasks[0].ID)+"/status", AgentTaskStatusRequest{
		NodeID:      registered.Node.ID,
		Status:      types.TaskRunning,
		ContainerID: "container-1",
	})
	if report.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, report.Code, report.Body.String())
	}
	var body TaskResponse
	decodeResponse(t, report, &body)
	if body.Task.ActualStatus != types.TaskRunning {
		t.Fatalf("expected running task, got %q", body.Task.ActualStatus)
	}
	if body.Task.ContainerID != "container-1" {
		t.Fatalf("expected container id to be recorded, got %q", body.Task.ContainerID)
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

func doAgentRequest(t *testing.T, handler http.Handler, method string, path string, body any) *httptest.ResponseRecorder {
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
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func registerTestNode(t *testing.T, handler http.Handler) AgentResponse {
	t.Helper()
	rec := doAgentRequest(t, handler, http.MethodPost, "/v1/agent/register", `{
		"node_name": "node-a",
		"advertise_address": "10.0.0.10",
		"labels": {"role": "app"},
		"capacity": {"cpu": 4000, "memory": 1024},
		"allocatable": {"cpu": 3000, "memory": 512}
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected register status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var registered AgentResponse
	decodeResponse(t, rec, &registered)
	return registered
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}
