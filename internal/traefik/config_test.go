package traefik

import (
	"testing"

	"github.com/alekpopovic/orch/internal/discovery"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestBuildConfigIncludesOnlyHealthyTCPRouteEndpoints(t *testing.T) {
	service := types.Service{
		ID:     "service-1",
		Status: types.ServiceActive,
		Spec: types.ServiceSpec{
			Name: "api",
			Routes: []types.Route{{
				Host:       "api.example.com",
				PathPrefix: "/",
				Port:       8080,
				TLS:        true,
			}},
		},
	}
	endpointSets := []discovery.ServiceEndpoints{{
		ServiceID:   service.ID,
		ServiceName: service.Spec.Name,
		Endpoints: []discovery.Endpoint{
			{
				ServiceID:      service.ID,
				ServiceName:    service.Spec.Name,
				TaskID:         "task-1",
				NodeID:         "node-1",
				NodeAddress:    "http://10.0.0.10:8081",
				PublicHostPort: 30001,
				ContainerPort:  8080,
				Protocol:       types.PortTCP,
				HealthStatus:   types.TaskRunning,
			},
			{
				ServiceID:      service.ID,
				ServiceName:    service.Spec.Name,
				TaskID:         "task-2",
				NodeID:         "node-1",
				NodeAddress:    "10.0.0.10",
				PublicHostPort: 30000,
				ContainerPort:  8080,
				Protocol:       types.PortTCP,
				HealthStatus:   types.TaskHealthy,
			},
			{
				ServiceID:      service.ID,
				ServiceName:    service.Spec.Name,
				TaskID:         "task-3",
				NodeID:         "node-1",
				NodeAddress:    "10.0.0.10",
				PublicHostPort: 30002,
				ContainerPort:  8080,
				Protocol:       types.PortTCP,
				HealthStatus:   types.TaskUnhealthy,
			},
			{
				ServiceID:      service.ID,
				ServiceName:    service.Spec.Name,
				TaskID:         "task-4",
				NodeID:         "node-1",
				NodeAddress:    "10.0.0.10",
				PublicHostPort: 30003,
				ContainerPort:  9090,
				Protocol:       types.PortTCP,
				HealthStatus:   types.TaskHealthy,
			},
			{
				ServiceID:      service.ID,
				ServiceName:    service.Spec.Name,
				TaskID:         "task-5",
				NodeID:         "node-1",
				NodeAddress:    "10.0.0.10",
				PublicHostPort: 30004,
				ContainerPort:  8080,
				Protocol:       types.PortUDP,
				HealthStatus:   types.TaskHealthy,
			},
		},
	}}

	config := BuildConfig([]types.Service{service}, endpointSets)

	router, ok := config.HTTP.Routers["orch-api-api-example-com-8080-0"]
	if !ok {
		t.Fatalf("expected route router, got %#v", config.HTTP.Routers)
	}
	if router.Rule != `Host("api.example.com") && PathPrefix("/")` {
		t.Fatalf("unexpected router rule %q", router.Rule)
	}
	if router.TLS == nil {
		t.Fatalf("expected TLS router config")
	}
	loadBalancer := config.HTTP.Services[router.Service].LoadBalancer
	if !loadBalancer.PassHostHeader {
		t.Fatalf("expected pass host header")
	}
	if len(loadBalancer.Servers) != 2 {
		t.Fatalf("expected two healthy TCP servers, got %#v", loadBalancer.Servers)
	}
	if loadBalancer.Servers[0].URL != "http://10.0.0.10:30000" || loadBalancer.Servers[1].URL != "http://10.0.0.10:30001" {
		t.Fatalf("unexpected servers %#v", loadBalancer.Servers)
	}
}

func TestBuildConfigSkipsRoutesWithoutHealthyServers(t *testing.T) {
	service := types.Service{
		ID:     "service-1",
		Status: types.ServiceActive,
		Spec: types.ServiceSpec{
			Name: "api",
			Routes: []types.Route{{
				Host:       "api.example.com",
				PathPrefix: "/",
				Port:       8080,
			}},
		},
	}
	endpointSets := []discovery.ServiceEndpoints{{
		ServiceID:   service.ID,
		ServiceName: service.Spec.Name,
		Endpoints: []discovery.Endpoint{{
			ServiceID:      service.ID,
			ServiceName:    service.Spec.Name,
			TaskID:         "task-1",
			NodeID:         "node-1",
			NodeAddress:    "10.0.0.10",
			PublicHostPort: 30000,
			ContainerPort:  8080,
			Protocol:       types.PortTCP,
			HealthStatus:   types.TaskUnhealthy,
		}},
	}}

	config := BuildConfig([]types.Service{service}, endpointSets)

	if len(config.HTTP.Routers) != 0 {
		t.Fatalf("expected no routers, got %#v", config.HTTP.Routers)
	}
	if len(config.HTTP.Services) != 0 {
		t.Fatalf("expected no services, got %#v", config.HTTP.Services)
	}
}
