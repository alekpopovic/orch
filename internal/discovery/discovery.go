package discovery

import (
	"sort"

	"github.com/alekpopovic/orch/pkg/types"
)

type Endpoint struct {
	ServiceID      types.ServiceID    `json:"service_id"`
	ServiceName    string             `json:"service_name"`
	TaskID         types.TaskID       `json:"task_id"`
	NodeID         types.NodeID       `json:"node_id"`
	NodeAddress    string             `json:"node_address"`
	PublicHostPort int                `json:"public_host_port"`
	ContainerPort  int                `json:"container_port"`
	Protocol       types.PortProtocol `json:"protocol"`
	HealthStatus   types.TaskStatus   `json:"health_status"`
	ServiceVersion int64              `json:"service_version"`
}

type ServiceEndpoints struct {
	ServiceID   types.ServiceID `json:"service_id"`
	ServiceName string          `json:"service_name"`
	Endpoints   []Endpoint      `json:"endpoints"`
}

func BuildServiceEndpoints(service types.Service, tasks []types.Task, nodes []types.Node, includeUnhealthy bool) ServiceEndpoints {
	nodesByID := make(map[types.NodeID]types.Node, len(nodes))
	for _, node := range nodes {
		nodesByID[node.ID] = node
	}
	endpoints := make([]Endpoint, 0)
	for _, task := range tasks {
		if task.ServiceID != service.ID || task.NodeID == "" {
			continue
		}
		if !includeTask(task, includeUnhealthy) {
			continue
		}
		node, ok := nodesByID[task.NodeID]
		if !ok {
			continue
		}
		for _, port := range taskPorts(task, service) {
			if port.ContainerPort == 0 {
				continue
			}
			endpoints = append(endpoints, Endpoint{
				ServiceID:      service.ID,
				ServiceName:    service.Spec.Name,
				TaskID:         task.ID,
				NodeID:         task.NodeID,
				NodeAddress:    node.AdvertiseAddress,
				PublicHostPort: port.PublishedPort,
				ContainerPort:  port.ContainerPort,
				Protocol:       port.Protocol,
				HealthStatus:   task.ActualStatus,
				ServiceVersion: task.Version,
			})
		}
	}
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].ServiceName != endpoints[j].ServiceName {
			return endpoints[i].ServiceName < endpoints[j].ServiceName
		}
		if endpoints[i].TaskID != endpoints[j].TaskID {
			return endpoints[i].TaskID < endpoints[j].TaskID
		}
		if endpoints[i].ContainerPort != endpoints[j].ContainerPort {
			return endpoints[i].ContainerPort < endpoints[j].ContainerPort
		}
		if endpoints[i].Protocol != endpoints[j].Protocol {
			return endpoints[i].Protocol < endpoints[j].Protocol
		}
		return endpoints[i].PublicHostPort < endpoints[j].PublicHostPort
	})
	return ServiceEndpoints{ServiceID: service.ID, ServiceName: service.Spec.Name, Endpoints: endpoints}
}

func BuildAllServiceEndpoints(services []types.Service, tasks []types.Task, nodes []types.Node, includeUnhealthy bool) []ServiceEndpoints {
	sort.Slice(services, func(i, j int) bool {
		if services[i].Spec.Name == services[j].Spec.Name {
			return services[i].ID < services[j].ID
		}
		return services[i].Spec.Name < services[j].Spec.Name
	})
	results := make([]ServiceEndpoints, 0, len(services))
	for _, service := range services {
		if service.Status != "" && service.Status != types.ServiceActive {
			continue
		}
		results = append(results, BuildServiceEndpoints(service, tasks, nodes, includeUnhealthy))
	}
	return results
}

func includeTask(task types.Task, includeUnhealthy bool) bool {
	if task.DesiredStatus != types.TaskRunning {
		return false
	}
	switch task.ActualStatus {
	case types.TaskRunning, types.TaskHealthy:
		return true
	case types.TaskUnhealthy:
		return includeUnhealthy
	default:
		return false
	}
}

func taskPorts(task types.Task, service types.Service) []types.Port {
	if len(task.Ports) > 0 {
		return task.Ports
	}
	return service.Spec.Ports
}
