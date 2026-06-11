package store

import (
	"context"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

type NodeStore interface {
	CreateNode(ctx context.Context, spec types.NodeSpec) (types.Node, error)
	GetNode(ctx context.Context, id types.NodeID) (types.Node, error)
	UpdateNode(ctx context.Context, node types.Node, expectedUpdatedAt time.Time) (types.Node, error)
	ListNodesByStatus(ctx context.Context, status types.NodeStatus) ([]types.Node, error)
}

type ServiceStore interface {
	CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error)
	GetService(ctx context.Context, id types.ServiceID) (types.Service, error)
	ListServices(ctx context.Context) ([]types.Service, error)
	UpdateService(ctx context.Context, id types.ServiceID, spec types.ServiceSpec, expectedUpdatedAt time.Time) (types.Service, error)
}

type TaskStore interface {
	CreateTask(ctx context.Context, task types.Task) (types.Task, error)
	GetTask(ctx context.Context, id types.TaskID) (types.Task, error)
	AssignTask(ctx context.Context, id types.TaskID, nodeID types.NodeID, expectedUpdatedAt time.Time) (types.Task, error)
	StopTask(ctx context.Context, id types.TaskID, expectedUpdatedAt time.Time) (types.Task, error)
	UpdateTaskStatus(ctx context.Context, id types.TaskID, desired types.TaskStatus, actual types.TaskStatus, containerID string, failureReason string, expectedUpdatedAt time.Time) (types.Task, error)
	ListTasksByService(ctx context.Context, serviceID types.ServiceID) ([]types.Task, error)
	ListTasksByNode(ctx context.Context, nodeID types.NodeID) ([]types.Task, error)
	ListTasksByStatus(ctx context.Context, status types.TaskStatus) ([]types.Task, error)
}

type DeploymentStore interface {
	CreateDeployment(ctx context.Context, deployment types.Deployment) (types.Deployment, error)
	GetDeployment(ctx context.Context, id types.DeploymentID) (types.Deployment, error)
	UpdateDeploymentStatus(ctx context.Context, id types.DeploymentID, status types.DeploymentStatus, expectedUpdatedAt time.Time) (types.Deployment, error)
}

type EventStore interface {
	AppendEvent(ctx context.Context, event types.Event) (types.Event, error)
	ListEvents(ctx context.Context, filter events.Filter) ([]types.Event, error)
}

type Store interface {
	NodeStore
	ServiceStore
	TaskStore
	DeploymentStore
	EventStore
}
