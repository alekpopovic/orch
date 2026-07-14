package cli

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/discovery"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestRootCommandConstruction(t *testing.T) {
	root := NewRootCommand(Options{})

	for _, path := range []string{
		"version",
		"node ls",
		"node inspect",
		"node drain",
		"node uncordon",
		"deploy",
		"service ls",
		"service inspect",
		"service ps",
		"scale",
		"rollout",
		"rollout status",
		"rollback",
		"delete",
		"endpoints",
		"events",
		"logs",
	} {
		if _, _, err := root.Find(strings.Fields(path)); err != nil {
			t.Fatalf("expected command %q: %v", path, err)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Options{Out: &out, Err: &bytes.Buffer{}})
	cmd.SetArgs([]string{"version"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if strings.TrimSpace(out.String()) != "orch dev" {
		t.Fatalf("unexpected output %q", out.String())
	}
}

func TestServerURLPrecedence(t *testing.T) {
	t.Setenv("ORCH_SERVER_URL", "http://env.example")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("server_url: http://config.example\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var gotServer string
	cmd := NewRootCommand(Options{
		Out:           &bytes.Buffer{},
		Err:           &bytes.Buffer{},
		DefaultConfig: configPath,
		NewClient: func(serverURL string) (Client, error) {
			gotServer = serverURL
			return &fakeClient{}, nil
		},
	})
	cmd.SetArgs([]string{"--server", "http://flag.example", "node", "ls"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute node ls: %v", err)
	}
	if gotServer != "http://flag.example" {
		t.Fatalf("expected flag server, got %q", gotServer)
	}
}

func TestDeployCommandSendsParsedSpec(t *testing.T) {
	deployPath := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(deployPath, []byte(`
name: api
image: ghcr.io/example/api:1.0.0
replicas: 3
ports:
  - container: 8080
resources:
  cpu: 500m
  memory: 512Mi
`), 0o600); err != nil {
		t.Fatalf("write deploy file: %v", err)
	}

	client := &fakeClient{}
	var out bytes.Buffer
	cmd := NewRootCommand(Options{
		Out: &out,
		Err: &bytes.Buffer{},
		NewClient: func(string) (Client, error) {
			return client, nil
		},
	})
	cmd.SetArgs([]string{"--server", "http://server.example", "deploy", deployPath})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute deploy: %v", err)
	}
	if client.created.Spec.Name != "api" || client.created.Spec.Replicas != 3 {
		t.Fatalf("unexpected created service %#v", client.created)
	}
	if !strings.Contains(out.String(), "api") {
		t.Fatalf("expected table output to contain service name, got %q", out.String())
	}
}

func TestScaleCommandResolvesServiceByName(t *testing.T) {
	client := &fakeClient{
		services: []types.Service{{
			ID:                "00000000-0000-4000-8000-000000000010",
			Spec:              types.ServiceSpec{Name: "api", Image: "nginx", Replicas: 1},
			DeploymentVersion: 1,
		}},
	}
	cmd := NewRootCommand(Options{
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
		NewClient: func(string) (Client, error) {
			return client, nil
		},
	})
	cmd.SetArgs([]string{"--server", "http://server.example", "scale", "api", "--replicas", "5"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scale: %v", err)
	}
	if client.scaledID != "00000000-0000-4000-8000-000000000010" || client.scaledReplicas != 5 {
		t.Fatalf("unexpected scale call id=%q replicas=%d", client.scaledID, client.scaledReplicas)
	}
}

func TestLogsCommandStreamsResolvedService(t *testing.T) {
	client := &fakeClient{
		services: []types.Service{{
			ID:                "00000000-0000-4000-8000-000000000010",
			Spec:              types.ServiceSpec{Name: "api", Image: "nginx", Replicas: 1},
			DeploymentVersion: 1,
		}},
	}
	cmd := NewRootCommand(Options{
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
		NewClient: func(string) (Client, error) {
			return client, nil
		},
	})
	cmd.SetArgs([]string{"--server", "http://server.example", "logs", "api", "--follow", "--task", "00000000-0000-4000-8000-000000000011", "--tail", "100"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute logs: %v", err)
	}
	if client.logServiceID != "00000000-0000-4000-8000-000000000010" || client.logTaskID != "00000000-0000-4000-8000-000000000011" {
		t.Fatalf("unexpected log target service=%q task=%q", client.logServiceID, client.logTaskID)
	}
	if !client.logFollow || client.logTail != "100" {
		t.Fatalf("unexpected log flags follow=%v tail=%q", client.logFollow, client.logTail)
	}
}

func TestEndpointsCommandResolvesService(t *testing.T) {
	client := &fakeClient{
		services: []types.Service{{
			ID:                "00000000-0000-4000-8000-000000000010",
			Spec:              types.ServiceSpec{Name: "api", Image: "nginx", Replicas: 1},
			DeploymentVersion: 1,
		}},
		endpoints: discovery.ServiceEndpoints{
			ServiceID:   "00000000-0000-4000-8000-000000000010",
			ServiceName: "api",
			Endpoints: []discovery.Endpoint{{
				ServiceName:    "api",
				TaskID:         "00000000-0000-4000-8000-000000000011",
				NodeID:         "00000000-0000-4000-8000-000000000001",
				NodeAddress:    "10.0.0.10",
				PublicHostPort: 18080,
				ContainerPort:  8080,
				Protocol:       types.PortTCP,
				HealthStatus:   types.TaskHealthy,
				ServiceVersion: 1,
			}},
		},
	}
	var out bytes.Buffer
	cmd := NewRootCommand(Options{
		Out: &out,
		Err: &bytes.Buffer{},
		NewClient: func(string) (Client, error) {
			return client, nil
		},
	})
	cmd.SetArgs([]string{"--server", "http://server.example", "endpoints", "api", "--include-unhealthy"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute endpoints: %v", err)
	}
	if client.endpointServiceID != "00000000-0000-4000-8000-000000000010" || !client.endpointIncludeUnhealthy {
		t.Fatalf("unexpected endpoint request id=%q include=%v", client.endpointServiceID, client.endpointIncludeUnhealthy)
	}
	if !strings.Contains(out.String(), "18080") || !strings.Contains(out.String(), "api") {
		t.Fatalf("expected endpoint table output, got %q", out.String())
	}
}

func TestEventsCommandFiltersByServiceName(t *testing.T) {
	client := &fakeClient{
		services: []types.Service{{
			ID:                "00000000-0000-4000-8000-000000000010",
			Spec:              types.ServiceSpec{Name: "api", Image: "nginx", Replicas: 1},
			DeploymentVersion: 1,
		}},
	}
	cmd := NewRootCommand(Options{
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
		NewClient: func(string) (Client, error) {
			return client, nil
		},
	})
	cmd.SetArgs([]string{"--server", "http://server.example", "events", "--service", "api"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute events: %v", err)
	}
	if client.eventFilter.ServiceID != "00000000-0000-4000-8000-000000000010" {
		t.Fatalf("expected service event filter, got %#v", client.eventFilter)
	}
}

func TestRolloutCommandSendsStrategyLimits(t *testing.T) {
	client := &fakeClient{
		services: []types.Service{{
			ID:                "00000000-0000-4000-8000-000000000010",
			Spec:              types.ServiceSpec{Name: "api", Image: "nginx", Replicas: 2},
			DeploymentVersion: 1,
		}},
	}
	cmd := NewRootCommand(Options{
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
		NewClient: func(string) (Client, error) {
			return client, nil
		},
	})
	cmd.SetArgs([]string{"--server", "http://server.example", "rollout", "api", "--image", "nginx:2", "--max-unavailable", "0", "--max-surge", "2"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute rollout: %v", err)
	}
	if client.rolloutID != "00000000-0000-4000-8000-000000000010" || client.rolloutImage != "nginx:2" {
		t.Fatalf("unexpected rollout target id=%q image=%q", client.rolloutID, client.rolloutImage)
	}
	if client.rolloutMaxUnavailable != 0 || client.rolloutMaxSurge != 2 {
		t.Fatalf("unexpected rollout limits maxUnavailable=%d maxSurge=%d", client.rolloutMaxUnavailable, client.rolloutMaxSurge)
	}
}

func TestRolloutStatusResolvesService(t *testing.T) {
	client := &fakeClient{
		services: []types.Service{{
			ID:                "00000000-0000-4000-8000-000000000010",
			Spec:              types.ServiceSpec{Name: "api", Image: "nginx", Replicas: 1},
			DeploymentVersion: 1,
		}},
	}
	cmd := NewRootCommand(Options{
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
		NewClient: func(string) (Client, error) {
			return client, nil
		},
	})
	cmd.SetArgs([]string{"--server", "http://server.example", "rollout", "status", "api"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute rollout status: %v", err)
	}
	if client.rolloutStatusID != "00000000-0000-4000-8000-000000000010" {
		t.Fatalf("unexpected rollout status id %q", client.rolloutStatusID)
	}
}

type fakeClient struct {
	services                 []types.Service
	created                  types.Service
	scaledID                 string
	scaledReplicas           int
	rolloutID                string
	rolloutImage             string
	rolloutMaxUnavailable    int
	rolloutMaxSurge          int
	rolloutStatusID          string
	logServiceID             string
	logTaskID                string
	logFollow                bool
	logTail                  string
	eventFilter              events.Filter
	endpointServiceID        string
	endpointIncludeUnhealthy bool
	endpoints                discovery.ServiceEndpoints
}

func (f *fakeClient) ListNodes(context.Context) ([]types.Node, error) {
	return []types.Node{{ID: "00000000-0000-4000-8000-000000000001", Hostname: "node-1", Status: types.NodeReady}}, nil
}

func (f *fakeClient) GetNode(_ context.Context, id string) (types.Node, error) {
	return types.Node{ID: types.NodeID(id), Hostname: "node-1", Status: types.NodeReady}, nil
}

func (f *fakeClient) DrainNode(_ context.Context, id string) (types.Node, error) {
	return types.Node{ID: types.NodeID(id), Status: types.NodeDraining}, nil
}

func (f *fakeClient) UncordonNode(_ context.Context, id string) (types.Node, error) {
	return types.Node{ID: types.NodeID(id), Status: types.NodeReady}, nil
}

func (f *fakeClient) CreateService(_ context.Context, spec types.ServiceSpec) (types.Service, error) {
	f.created = types.Service{
		ID:                "00000000-0000-4000-8000-000000000020",
		Spec:              spec,
		DeploymentVersion: 1,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	return f.created, nil
}

func (f *fakeClient) ListServices(context.Context) ([]types.Service, error) {
	return f.services, nil
}

func (f *fakeClient) GetService(_ context.Context, id string) (types.Service, error) {
	for _, service := range f.services {
		if string(service.ID) == id {
			return service, nil
		}
	}
	return types.Service{ID: types.ServiceID(id), Spec: types.ServiceSpec{Name: "api", Image: "nginx"}}, nil
}

func (f *fakeClient) GetServiceEndpoints(_ context.Context, id string, includeUnhealthy bool) (discovery.ServiceEndpoints, error) {
	f.endpointServiceID = id
	f.endpointIncludeUnhealthy = includeUnhealthy
	return f.endpoints, nil
}

func (f *fakeClient) DeleteService(context.Context, string) error {
	return nil
}

func (f *fakeClient) ScaleService(_ context.Context, id string, replicas int) (types.Service, error) {
	f.scaledID = id
	f.scaledReplicas = replicas
	return types.Service{ID: types.ServiceID(id), Spec: types.ServiceSpec{Name: "api", Image: "nginx", Replicas: replicas}}, nil
}

func (f *fakeClient) RolloutService(_ context.Context, id string, image string, maxUnavailable int, maxSurge int) (types.Deployment, error) {
	f.rolloutID = id
	f.rolloutImage = image
	f.rolloutMaxUnavailable = maxUnavailable
	f.rolloutMaxSurge = maxSurge
	return types.Deployment{ID: "00000000-0000-4000-8000-000000000030", ServiceID: types.ServiceID(id)}, nil
}

func (f *fakeClient) GetServiceRollout(_ context.Context, id string) (types.Deployment, error) {
	f.rolloutStatusID = id
	return types.Deployment{ID: "00000000-0000-4000-8000-000000000030", ServiceID: types.ServiceID(id), Status: types.DeploymentRunning}, nil
}

func (f *fakeClient) RollbackService(_ context.Context, id string) (types.Deployment, error) {
	return types.Deployment{ID: "00000000-0000-4000-8000-000000000031", ServiceID: types.ServiceID(id)}, nil
}

func (f *fakeClient) ListTasks(context.Context, url.Values) ([]types.Task, error) {
	return nil, nil
}

func (f *fakeClient) GetTask(_ context.Context, id string) (types.Task, error) {
	return types.Task{ID: types.TaskID(id)}, nil
}

func (f *fakeClient) ListEvents(_ context.Context, filter events.Filter) ([]types.Event, error) {
	f.eventFilter = filter
	return nil, nil
}

func (f *fakeClient) StreamLogs(_ context.Context, serviceID string, taskID string, follow bool, tail string, _ io.Writer) error {
	f.logServiceID = serviceID
	f.logTaskID = taskID
	f.logFollow = follow
	f.logTail = tail
	return nil
}
