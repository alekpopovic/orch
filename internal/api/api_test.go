package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/auth"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/metrics"
	"github.com/alekpopovic/orch/internal/traefik"
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

func TestMetricsEndpoint(t *testing.T) {
	serverMetrics := metrics.NewServer()
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithRequestMetrics(serverMetrics), WithMetricsHandler(serverMetrics.Handler()))

	health := doRequest(t, handler, http.MethodGet, "/healthz", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("expected health status %d, got %d", http.StatusOK, health.Code)
	}
	rec := doRequest(t, handler, http.MethodGet, "/metrics", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "api_requests_total") {
		t.Fatalf("expected API request metrics, got %s", rec.Body.String())
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
	if registered.Credential == nil || registered.Credential.Token == "" {
		t.Fatalf("expected issued agent credential")
	}

	heartbeat := doAgentCredentialRequest(t, handler, http.MethodPost, "/v1/agent/heartbeat", `{
		"node_id": "`+string(registered.Node.ID)+`",
		"capacity": {"cpu": 4000, "memory": 1024},
		"allocatable": {"cpu": 3000, "memory": 512}
	}`, registered.Credential.Token)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, heartbeat.Code, heartbeat.Body.String())
	}
	var acknowledged AgentResponse
	decodeResponse(t, heartbeat, &acknowledged)
	if acknowledged.Node.LastHeartbeatAt.IsZero() {
		t.Fatalf("expected heartbeat timestamp")
	}
	if acknowledged.Credential == nil || acknowledged.Credential.Token == "" || acknowledged.Credential.Token == registered.Credential.Token {
		t.Fatalf("expected rotated heartbeat credential")
	}
	oldCredentialHeartbeat := doAgentCredentialRequest(t, handler, http.MethodPost, "/v1/agent/heartbeat", `{
		"node_id": "`+string(registered.Node.ID)+`",
		"capacity": {"cpu": 4000, "memory": 1024},
		"allocatable": {"cpu": 3000, "memory": 512}
	}`, registered.Credential.Token)
	if oldCredentialHeartbeat.Code != http.StatusUnauthorized {
		t.Fatalf("expected old credential to be rejected, got %d", oldCredentialHeartbeat.Code)
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

func TestSecretsCreateRetrieveMetadataAndRedactPlaintext(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"))
	const secretValue = "postgres://user:pass@db/app"

	create := doRequest(t, handler, http.MethodPost, "/v1/secrets", CreateSecretRequest{
		Name:  "prod/database-url",
		Value: secretValue,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, create.Code, create.Body.String())
	}
	if strings.Contains(create.Body.String(), secretValue) {
		t.Fatalf("create response leaked secret plaintext: %s", create.Body.String())
	}
	var created SecretResponse
	decodeResponse(t, create, &created)
	if created.Secret.Name != "prod/database-url" || created.Secret.CreatedAt.IsZero() {
		t.Fatalf("unexpected secret metadata %#v", created.Secret)
	}

	get := doRequest(t, handler, http.MethodGet, "/v1/secrets/"+url.PathEscape("prod/database-url"), nil)
	if get.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, get.Code, get.Body.String())
	}
	if strings.Contains(get.Body.String(), secretValue) {
		t.Fatalf("get response leaked secret plaintext: %s", get.Body.String())
	}
	var got SecretResponse
	decodeResponse(t, get, &got)
	if got.Secret.Name != created.Secret.Name {
		t.Fatalf("expected secret %q, got %q", created.Secret.Name, got.Secret.Name)
	}

	list := doRequest(t, handler, http.MethodGet, "/v1/secrets", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), secretValue) {
		t.Fatalf("list response leaked secret plaintext: %s", list.Body.String())
	}
	events := doRequest(t, handler, http.MethodGet, "/v1/events?type=secret.created", nil)
	if events.Code != http.StatusOK {
		t.Fatalf("expected events status %d, got %d: %s", http.StatusOK, events.Code, events.Body.String())
	}
	if strings.Contains(events.Body.String(), secretValue) {
		t.Fatalf("event response leaked secret plaintext: %s", events.Body.String())
	}
}

func TestSecretInjectedIntoAgentTaskEnv(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"))
	registered := registerTestNode(t, handler)
	const secretValue = "postgres://user:pass@db/app"
	createSecret := doRequest(t, handler, http.MethodPost, "/v1/secrets", CreateSecretRequest{
		Name:  "prod/database-url",
		Value: secretValue,
	})
	if createSecret.Code != http.StatusCreated {
		t.Fatalf("expected secret status %d, got %d: %s", http.StatusCreated, createSecret.Code, createSecret.Body.String())
	}
	createService := doRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "secret-api",
			"image": "nginx:1.27",
			"replicas": 1,
			"env": {"NODE_ENV": "production"},
			"secret_refs": [{"name": "prod/database-url", "env": "DATABASE_URL"}],
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`)
	if createService.Code != http.StatusCreated {
		t.Fatalf("expected service status %d, got %d: %s", http.StatusCreated, createService.Code, createService.Body.String())
	}

	list := doAgentCredentialRequest(t, handler, http.MethodGet, "/v1/agent/tasks?node_id="+string(registered.Node.ID), nil, registered.Credential.Token)
	if list.Code != http.StatusOK {
		t.Fatalf("expected tasks status %d, got %d: %s", http.StatusOK, list.Code, list.Body.String())
	}
	var tasks AgentTasksResponse
	decodeResponse(t, list, &tasks)
	if len(tasks.Tasks) != 1 {
		t.Fatalf("expected one assigned task, got %#v", tasks.Tasks)
	}
	if tasks.Tasks[0].Env["DATABASE_URL"] != secretValue {
		t.Fatalf("expected secret env to be injected, got %#v", tasks.Tasks[0].Env)
	}
	if tasks.Tasks[0].Env["NODE_ENV"] != "production" {
		t.Fatalf("expected literal env to be preserved, got %#v", tasks.Tasks[0].Env)
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

	list := doAgentCredentialRequest(t, handler, http.MethodGet, "/v1/agent/tasks?node_id="+string(registered.Node.ID), nil, registered.Credential.Token)
	if list.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, list.Code, list.Body.String())
	}
	var tasks AgentTasksResponse
	decodeResponse(t, list, &tasks)
	if len(tasks.Tasks) != 1 {
		t.Fatalf("expected one assigned task, got %d", len(tasks.Tasks))
	}

	report := doAgentCredentialRequest(t, handler, http.MethodPost, "/v1/agent/tasks/"+string(tasks.Tasks[0].Task.ID)+"/status", AgentTaskStatusRequest{
		NodeID:      registered.Node.ID,
		Status:      types.TaskRunning,
		ContainerID: "container-1",
	}, registered.Credential.Token)
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

func TestStreamLogsCancellation(t *testing.T) {
	streamer := &blockingLogStreamer{started: make(chan struct{}), done: make(chan struct{})}
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"), WithLogStreamer(streamer))
	registered := registerTestNode(t, handler)
	created := createLogTestService(t, handler)
	tasks := listAgentTasks(t, handler, registered)
	report := doAgentCredentialRequest(t, handler, http.MethodPost, "/v1/agent/tasks/"+string(tasks.Tasks[0].Task.ID)+"/status", AgentTaskStatusRequest{
		NodeID:      registered.Node.ID,
		Status:      types.TaskRunning,
		ContainerID: "container-1",
	}, registered.Credential.Token)
	if report.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, report.Code, report.Body.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/logs?service_id="+string(created.Service.ID)+"&follow=true", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	go func() {
		handler.ServeHTTP(rec, req)
		close(streamer.done)
	}()
	<-streamer.started
	cancel()

	select {
	case <-streamer.done:
	case <-time.After(time.Second):
		t.Fatalf("log stream did not stop after cancellation")
	}
}

func TestStreamLogsRejectsOfflineNode(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithBootstrapToken("secret"))
	registered := registerTestNode(t, handler)
	created := createLogTestService(t, handler)
	tasks := listAgentTasks(t, handler, registered)
	report := doAgentCredentialRequest(t, handler, http.MethodPost, "/v1/agent/tasks/"+string(tasks.Tasks[0].Task.ID)+"/status", AgentTaskStatusRequest{
		NodeID:      registered.Node.ID,
		Status:      types.TaskRunning,
		ContainerID: "container-1",
	}, registered.Credential.Token)
	if report.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, report.Code, report.Body.String())
	}
	heartbeat := doAgentCredentialRequest(t, handler, http.MethodPost, "/v1/agent/heartbeat", `{
		"node_id": "`+string(registered.Node.ID)+`",
		"shutdown": true
	}`, registered.Credential.Token)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("expected heartbeat status %d, got %d: %s", http.StatusOK, heartbeat.Code, heartbeat.Body.String())
	}

	rec := doRequest(t, handler, http.MethodGet, "/v1/logs?service_id="+string(created.Service.ID), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
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

func TestServiceDiscoveryEndpointsFilterUnhealthyTasks(t *testing.T) {
	controlPlane := controlplane.NewMemoryService()
	handler := NewHandler(slog.Default(), controlPlane, WithBootstrapToken("secret"))
	registered := registerTestNode(t, handler)
	service, err := controlPlane.CreateService(context.Background(), types.ServiceSpec{
		Name:     "api",
		Image:    "nginx:1.27",
		Replicas: 3,
		Ports:    []types.Port{{Protocol: types.PortTCP, ContainerPort: 8080}},
		ResourceRequirements: types.ResourceRequirements{
			Requests: types.Resources{CPU: 100, Memory: 128},
			Limits:   types.Resources{CPU: 100, Memory: 128},
		},
		RestartPolicy: types.RestartPolicy{Condition: types.RestartNever},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	tasks, err := controlPlane.ListTasks(context.Background(), controlplane.TaskFilter{ServiceID: service.ID})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected three assigned tasks, got %#v", tasks)
	}
	for i, status := range []types.TaskStatus{types.TaskRunning, types.TaskHealthy, types.TaskUnhealthy} {
		if _, err := controlPlane.ReportTaskStatus(context.Background(), controlplane.TaskStatusReport{
			TaskID:      tasks[i].ID,
			NodeID:      registered.Node.ID,
			Status:      status,
			ContainerID: "container-" + string(rune('a'+i)),
		}); err != nil {
			t.Fatalf("report task status %s: %v", status, err)
		}
	}

	rec := doRequest(t, handler, http.MethodGet, "/v1/services/"+string(service.ID)+"/endpoints", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var body ServiceEndpointsResponse
	decodeResponse(t, rec, &body)
	if body.ServiceName != "api" || len(body.Endpoints) != 2 {
		t.Fatalf("expected two healthy endpoints for api, got %#v", body)
	}
	if body.Endpoints[0].NodeAddress != "10.0.0.10" || body.Endpoints[0].PublicHostPort == 0 {
		t.Fatalf("expected node address and assigned public port, got %#v", body.Endpoints[0])
	}

	rec = doRequest(t, handler, http.MethodGet, "/v1/discovery/services/api?include_unhealthy=true", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	decodeResponse(t, rec, &body)
	if len(body.Endpoints) != 3 {
		t.Fatalf("expected unhealthy endpoint when requested, got %#v", body.Endpoints)
	}

	all := doRequest(t, handler, http.MethodGet, "/v1/discovery/services", nil)
	if all.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, all.Code, all.Body.String())
	}
	var allBody DiscoveryServicesResponse
	decodeResponse(t, all, &allBody)
	if len(allBody.Services) != 1 || allBody.Services[0].ServiceName != "api" {
		t.Fatalf("expected api discovery service, got %#v", allBody)
	}
}

func TestTraefikConfigUsesHealthyServiceRouteEndpoints(t *testing.T) {
	controlPlane := controlplane.NewMemoryService()
	handler := NewHandler(slog.Default(), controlPlane, WithBootstrapToken("secret"))
	registered := registerTestNode(t, handler)
	service, err := controlPlane.CreateService(context.Background(), types.ServiceSpec{
		Name:     "api",
		Image:    "nginx:1.27",
		Replicas: 3,
		Ports:    []types.Port{{Protocol: types.PortTCP, ContainerPort: 8080}},
		Routes: []types.Route{{
			Host:       "api.localhost",
			PathPrefix: "/",
			Port:       8080,
			TLS:        true,
		}},
		ResourceRequirements: types.ResourceRequirements{
			Requests: types.Resources{CPU: 100, Memory: 128},
			Limits:   types.Resources{CPU: 100, Memory: 128},
		},
		RestartPolicy: types.RestartPolicy{Condition: types.RestartNever},
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	tasks, err := controlPlane.ListTasks(context.Background(), controlplane.TaskFilter{ServiceID: service.ID})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected three assigned tasks, got %#v", tasks)
	}
	for taskIndex, status := range []types.TaskStatus{types.TaskRunning, types.TaskHealthy, types.TaskUnhealthy} {
		if _, err := controlPlane.ReportTaskStatus(context.Background(), controlplane.TaskStatusReport{
			TaskID:      tasks[taskIndex].ID,
			NodeID:      registered.Node.ID,
			Status:      status,
			ContainerID: "container-" + string(rune('a'+taskIndex)),
		}); err != nil {
			t.Fatalf("report task status %s: %v", status, err)
		}
	}

	rec := doRequest(t, handler, http.MethodGet, "/v1/integrations/traefik/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var body traefik.Config
	decodeResponse(t, rec, &body)
	if len(body.HTTP.Routers) != 1 {
		t.Fatalf("expected one Traefik router, got %#v", body.HTTP.Routers)
	}
	for _, router := range body.HTTP.Routers {
		if router.Rule != `Host("api.localhost") && PathPrefix("/")` {
			t.Fatalf("unexpected router rule %q", router.Rule)
		}
		if router.TLS == nil {
			t.Fatalf("expected TLS router config")
		}
		loadBalancer := body.HTTP.Services[router.Service].LoadBalancer
		if len(loadBalancer.Servers) != 2 {
			t.Fatalf("expected only healthy running servers, got %#v", loadBalancer.Servers)
		}
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

func TestUserAuthMissingToken(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithUserJWT("test-secret"))
	rec := doRequest(t, handler, http.MethodGet, "/v1/services", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestUserAuthInvalidToken(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithUserJWT("test-secret"))
	rec := doAuthenticatedRequest(t, handler, http.MethodGet, "/v1/services", nil, "not-a-jwt")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestUserAuthInsufficientRole(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithUserJWT("test-secret"))
	token := userToken(t, auth.RoleViewer)
	rec := doAuthenticatedRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "api",
			"image": "nginx:1.27",
			"replicas": 1,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`, token)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

func TestUserAuthAllowedAccess(t *testing.T) {
	handler := NewHandler(slog.Default(), controlplane.NewMemoryService(), WithUserJWT("test-secret"))
	token := userToken(t, auth.RoleOperator)
	rec := doAuthenticatedRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "api",
			"image": "nginx:1.27",
			"replicas": 1,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`, token)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
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

func TestStartAndGetRollout(t *testing.T) {
	handler := newTestHandler()
	create := doRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "api",
			"image": "nginx:1.27",
			"replicas": 2,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`)
	var created ServiceResponse
	decodeResponse(t, create, &created)

	start := doRequest(t, handler, http.MethodPost, "/v1/services/"+string(created.Service.ID)+"/rollout", `{
		"image": "nginx:1.28",
		"maxUnavailable": 0,
		"maxSurge": 2
	}`)
	if start.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, start.Code, start.Body.String())
	}
	var started DeploymentResponse
	decodeResponse(t, start, &started)
	if started.Deployment.Status != types.DeploymentPending {
		t.Fatalf("expected pending deployment, got %q", started.Deployment.Status)
	}
	if started.Deployment.MaxUnavailable != 0 || started.Deployment.MaxSurge != 2 {
		t.Fatalf("unexpected rollout limits %#v", started.Deployment)
	}

	latest := doRequest(t, handler, http.MethodGet, "/v1/services/"+string(created.Service.ID)+"/rollout", nil)
	if latest.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, latest.Code, latest.Body.String())
	}
	var latestBody DeploymentResponse
	decodeResponse(t, latest, &latestBody)
	if latestBody.Deployment.ID != started.Deployment.ID {
		t.Fatalf("expected latest rollout %q, got %q", started.Deployment.ID, latestBody.Deployment.ID)
	}

	byID := doRequest(t, handler, http.MethodGet, "/v1/rollouts/"+string(started.Deployment.ID), nil)
	if byID.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, byID.Code, byID.Body.String())
	}
}

func TestListEvents(t *testing.T) {
	handler := newTestHandler()
	create := doRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "events",
			"image": "nginx:1.27",
			"replicas": 1,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`)
	var created ServiceResponse
	decodeResponse(t, create, &created)

	rec := doRequest(t, handler, http.MethodGet, "/v1/events?service_id="+string(created.Service.ID)+"&type=service.created&severity=info&limit=10", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var body ListEventsResponse
	decodeResponse(t, rec, &body)
	if len(body.Events) == 0 {
		t.Fatalf("expected at least one event")
	}
	if body.Events[0].Type != "service.created" {
		t.Fatalf("expected service.created event, got %q", body.Events[0].Type)
	}
}

func TestListEventsInvalidFilters(t *testing.T) {
	tests := []string{
		"/v1/events?severity=warn",
		"/v1/events?since=not-a-time",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			rec := doRequest(t, newTestHandler(), http.MethodGet, path, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
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

func doAuthenticatedRequest(t *testing.T, handler http.Handler, method string, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := requestWithHeaders(t, handler, method, path, body, map[string]string{"Authorization": "Bearer " + token})
	return recorder
}

func requestWithHeaders(t *testing.T, handler http.Handler, method string, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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
	for key, value := range headers {
		req.Header.Set(key, value)
	}
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

func doAgentCredentialRequest(t *testing.T, handler http.Handler, method string, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	return requestWithHeaders(t, handler, method, path, body, map[string]string{"Authorization": "Bearer " + token})
}

func userToken(t *testing.T, role auth.Role) string {
	t.Helper()
	token, err := auth.SignJWT(auth.Claims{
		Subject:   "user-1",
		Role:      role,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, "test-secret")
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return token
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
	if registered.Credential == nil || registered.Credential.Token == "" {
		t.Fatalf("expected issued agent credential")
	}
	return registered
}

func createLogTestService(t *testing.T, handler http.Handler) ServiceResponse {
	t.Helper()
	create := doRequest(t, handler, http.MethodPost, "/v1/services", `{
		"spec": {
			"name": "logs-api",
			"image": "nginx:1.27",
			"replicas": 1,
			"resource_requirements": {"requests": {}, "limits": {}}
		}
	}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, create.Code, create.Body.String())
	}
	var created ServiceResponse
	decodeResponse(t, create, &created)
	return created
}

func listAgentTasks(t *testing.T, handler http.Handler, registered AgentResponse) AgentTasksResponse {
	t.Helper()
	list := doAgentCredentialRequest(t, handler, http.MethodGet, "/v1/agent/tasks?node_id="+string(registered.Node.ID), nil, registered.Credential.Token)
	if list.Code != http.StatusOK {
		t.Fatalf("expected tasks status %d, got %d: %s", http.StatusOK, list.Code, list.Body.String())
	}
	var tasks AgentTasksResponse
	decodeResponse(t, list, &tasks)
	if len(tasks.Tasks) == 0 {
		t.Fatalf("expected assigned tasks")
	}
	return tasks
}

type blockingLogStreamer struct {
	started chan struct{}
	done    chan struct{}
}

func (s *blockingLogStreamer) StreamLogs(ctx context.Context, _ LogStreamRequest, _ io.Writer) error {
	close(s.started)
	<-ctx.Done()
	return ctx.Err()
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}
