package store

import (
	"context"
	"fmt"
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

type NamespaceStore interface {
	CreateNamespace(ctx context.Context, name string) (types.Namespace, error)
	ListNamespaces(ctx context.Context) ([]types.Namespace, error)
	DeleteNamespace(ctx context.Context, name string) error
}

type ResourceQuotaStore interface {
	GetResourceQuota(ctx context.Context) (types.ResourceQuota, types.ResourceUsage, error)
	SetResourceQuota(ctx context.Context, value types.ResourceQuota) (types.ResourceQuota, types.ResourceUsage, error)
}

type GitOpsStore interface {
	CreateGitOpsSource(ctx context.Context, source types.GitOpsSource) (types.GitOpsSource, error)
	ListGitOpsSources(ctx context.Context) ([]types.GitOpsSource, error)
	GetGitOpsSource(ctx context.Context, id string) (types.GitOpsSource, error)
	UpdateGitOpsSource(ctx context.Context, source types.GitOpsSource) (types.GitOpsSource, error)
	DeleteGitOpsSource(ctx context.Context, id string) error
}

type ServiceStore interface {
	CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error)
	GetService(ctx context.Context, id types.ServiceID) (types.Service, error)
	ListServices(ctx context.Context) ([]types.Service, error)
	UpdateService(ctx context.Context, id types.ServiceID, spec types.ServiceSpec, expectedUpdatedAt time.Time) (types.Service, error)
	UpdateServiceStatus(ctx context.Context, id types.ServiceID, status types.ServiceStatus, expectedUpdatedAt time.Time) (types.Service, error)
}

type TaskStore interface {
	CreateTask(ctx context.Context, task types.Task) (types.Task, error)
	GetTask(ctx context.Context, id types.TaskID) (types.Task, error)
	AssignTask(ctx context.Context, id types.TaskID, nodeID types.NodeID, ports []types.Port, expectedUpdatedAt time.Time) (types.Task, error)
	StopTask(ctx context.Context, id types.TaskID, expectedUpdatedAt time.Time) (types.Task, error)
	UpdateTaskStatus(ctx context.Context, id types.TaskID, desired types.TaskStatus, actual types.TaskStatus, containerID string, failureReason string, expectedUpdatedAt time.Time) (types.Task, error)
	ListTasksByService(ctx context.Context, serviceID types.ServiceID) ([]types.Task, error)
	ListTasksByNode(ctx context.Context, nodeID types.NodeID) ([]types.Task, error)
	ListTasksByStatus(ctx context.Context, status types.TaskStatus) ([]types.Task, error)
}

type DeploymentStore interface {
	CreateDeployment(ctx context.Context, deployment types.Deployment) (types.Deployment, error)
	GetDeployment(ctx context.Context, id types.DeploymentID) (types.Deployment, error)
	ListDeploymentsByStatus(ctx context.Context, status types.DeploymentStatus) ([]types.Deployment, error)
	UpdateDeploymentStatus(ctx context.Context, id types.DeploymentID, status types.DeploymentStatus, expectedUpdatedAt time.Time) (types.Deployment, error)
}

type EventStore interface {
	AppendEvent(ctx context.Context, event types.Event) (types.Event, error)
	ListEvents(ctx context.Context, filter events.Filter) ([]types.Event, error)
}

type SecretStore interface {
	CreateSecret(ctx context.Context, secret types.Secret) (types.Secret, error)
	GetSecret(ctx context.Context, name string) (types.Secret, error)
	ListSecrets(ctx context.Context) ([]types.Secret, error)
	DeleteSecret(ctx context.Context, name string) error
}

type RegistryCredentialStore interface {
	CreateRegistryCredential(ctx context.Context, credential types.RegistryCredential) (types.RegistryCredential, error)
	GetRegistryCredential(ctx context.Context, id string) (types.RegistryCredential, error)
	ListRegistryCredentials(ctx context.Context) ([]types.RegistryCredential, error)
	DeleteRegistryCredential(ctx context.Context, id string) error
}

type Store interface {
	NamespaceStore
	ResourceQuotaStore
	GitOpsStore
	NodeStore
	ServiceStore
	TaskStore
	DeploymentStore
	EventStore
	SecretStore
	RegistryCredentialStore
}

// TxFunc runs a set of store operations inside one transaction boundary.
type TxFunc func(ctx context.Context, tx any) error

// Transactor is implemented by stores that can execute a callback atomically.
type Transactor interface {
	WithTx(ctx context.Context, fn TxFunc) error
}

// WithTx runs fn inside candidate's transaction support when it exists.
func WithTx[S any](ctx context.Context, candidate S, fn func(context.Context, S) error) error {
	transactor, ok := any(candidate).(Transactor)
	if !ok {
		return fn(ctx, candidate)
	}
	return transactor.WithTx(ctx, func(txCtx context.Context, tx any) error {
		scoped, ok := any(tx).(S)
		if !ok {
			return fmt.Errorf("%w: transaction store does not implement requested boundary", ErrInvalidState)
		}
		return fn(txCtx, scoped)
	})
}
