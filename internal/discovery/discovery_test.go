package discovery

import (
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestBuildServiceEndpointsFiltersByHealth(t *testing.T) {
	service := types.Service{
		ID: "svc-1",
		Spec: types.ServiceSpec{
			Name:  "api",
			Ports: []types.Port{{Protocol: types.PortTCP, ContainerPort: 8080, PublishedPort: 18080}},
		},
	}
	nodes := []types.Node{{ID: "node-a", AdvertiseAddress: "10.0.0.10"}}
	tasks := []types.Task{
		endpointTask("running", "node-a", types.TaskRunning),
		endpointTask("healthy", "node-a", types.TaskHealthy),
		endpointTask("unhealthy", "node-a", types.TaskUnhealthy),
		endpointTask("pending", "node-a", types.TaskPending),
	}

	defaultEndpoints := BuildServiceEndpoints(service, tasks, nodes, false)
	if got := len(defaultEndpoints.Endpoints); got != 2 {
		t.Fatalf("expected running and healthy endpoints, got %d: %#v", got, defaultEndpoints.Endpoints)
	}

	withUnhealthy := BuildServiceEndpoints(service, tasks, nodes, true)
	if got := len(withUnhealthy.Endpoints); got != 3 {
		t.Fatalf("expected unhealthy endpoint to be included, got %d: %#v", got, withUnhealthy.Endpoints)
	}
	if withUnhealthy.Endpoints[2].HealthStatus != types.TaskUnhealthy {
		t.Fatalf("expected unhealthy endpoint last, got %#v", withUnhealthy.Endpoints)
	}
}

func TestBuildServiceEndpointsUsesTaskAssignedPorts(t *testing.T) {
	service := types.Service{
		ID: "svc-1",
		Spec: types.ServiceSpec{
			Name:  "api",
			Ports: []types.Port{{Protocol: types.PortTCP, ContainerPort: 8080, PublishedPort: 18080}},
		},
	}
	task := endpointTask("task-a", "node-a", types.TaskHealthy)
	task.Ports = []types.Port{{Protocol: types.PortTCP, ContainerPort: 8080, PublishedPort: 30000}}

	endpoints := BuildServiceEndpoints(service, []types.Task{task}, []types.Node{{ID: "node-a", AdvertiseAddress: "10.0.0.10"}}, false)
	if len(endpoints.Endpoints) != 1 {
		t.Fatalf("expected one endpoint, got %#v", endpoints.Endpoints)
	}
	if endpoints.Endpoints[0].PublicHostPort != 30000 {
		t.Fatalf("expected task assigned port, got %#v", endpoints.Endpoints[0])
	}
}

func endpointTask(id types.TaskID, nodeID types.NodeID, status types.TaskStatus) types.Task {
	return types.Task{
		ID:            id,
		ServiceID:     "svc-1",
		NodeID:        nodeID,
		DesiredStatus: types.TaskRunning,
		ActualStatus:  status,
		Version:       7,
	}
}
