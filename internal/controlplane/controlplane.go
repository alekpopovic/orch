package controlplane

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/secrets"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

type Service interface {
	RegisterNode(ctx context.Context, registration NodeRegistration) (NodeCommand, error)
	HeartbeatNode(ctx context.Context, heartbeat NodeHeartbeat) (NodeCommand, error)
	MarkStaleNodesOffline(ctx context.Context, timeout time.Duration) ([]types.Node, error)
	SetAgentCredential(ctx context.Context, nodeID types.NodeID, tokenHash string, expiresAt time.Time) (types.Node, error)
	RevokeNode(ctx context.Context, nodeID types.NodeID) (types.Node, error)
	ListAssignedTasks(ctx context.Context, nodeID types.NodeID) ([]AgentTask, error)
	ReportTaskStatus(ctx context.Context, report TaskStatusReport) (types.Task, error)
	ListNodes(ctx context.Context) ([]types.Node, error)
	GetNode(ctx context.Context, id types.NodeID) (types.Node, error)
	DrainNode(ctx context.Context, id types.NodeID) (types.Node, error)
	UncordonNode(ctx context.Context, id types.NodeID) (types.Node, error)
	GetNodeDrainStatus(ctx context.Context, id types.NodeID) (NodeDrainStatus, error)
	CreateSecret(ctx context.Context, name string, plaintext string) (types.SecretMetadata, error)
	ListSecrets(ctx context.Context) ([]types.SecretMetadata, error)
	GetSecret(ctx context.Context, name string) (types.SecretMetadata, error)
	DeleteSecret(ctx context.Context, name string) error
	CreateRegistryCredential(ctx context.Context, spec RegistryCredentialSpec) (types.RegistryCredentialMetadata, error)
	ListRegistryCredentials(ctx context.Context) ([]types.RegistryCredentialMetadata, error)
	DeleteRegistryCredential(ctx context.Context, id string) error
	CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error)
	ListServices(ctx context.Context) ([]types.Service, error)
	GetService(ctx context.Context, id types.ServiceID) (types.Service, error)
	DeleteService(ctx context.Context, id types.ServiceID) error
	ScaleService(ctx context.Context, id types.ServiceID, replicas int) (types.Service, error)
	RolloutService(ctx context.Context, id types.ServiceID, spec RolloutSpec) (types.Deployment, error)
	GetDeployment(ctx context.Context, id types.DeploymentID) (types.Deployment, error)
	GetServiceRollout(ctx context.Context, id types.ServiceID) (types.Deployment, error)
	RollbackService(ctx context.Context, id types.ServiceID) (types.Deployment, error)
	ListTasks(ctx context.Context, filter TaskFilter) ([]types.Task, error)
	GetTask(ctx context.Context, id types.TaskID) (types.Task, error)
	ListEvents(ctx context.Context, filter events.Filter) ([]types.Event, error)
}

type NodeRegistration struct {
	Name             string
	AdvertiseAddress string
	Labels           map[string]string
	Capacity         types.Resources
	Allocatable      types.Resources
}

type NodeHeartbeat struct {
	NodeID      types.NodeID
	Capacity    types.Resources
	Allocatable types.Resources
	Labels      map[string]string
	Shutdown    bool
}

type NodeCommand struct {
	Node       types.Node
	Directives []AgentDirective
}

type AgentDirective struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

type AgentTask struct {
	Task          types.Task         `json:"task"`
	Healthcheck   *types.Healthcheck `json:"healthcheck,omitempty"`
	Ports         []types.Port       `json:"ports,omitempty"`
	Env           map[string]string  `json:"env,omitempty"`
	ImagePullAuth *RegistryAuth      `json:"image_pull_auth,omitempty"`
}

type RegistryCredentialSpec struct {
	ID       string
	Registry string
	Username string
	Password string
}

type RegistryAuth struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	ServerAddress string `json:"server_address,omitempty"`
}

type DrainPhase string

const (
	DrainNotDraining DrainPhase = "not_draining"
	DrainPending     DrainPhase = "pending"
	DrainComplete    DrainPhase = "complete"
	DrainOffline     DrainPhase = "offline"
)

type NodeDrainStatus struct {
	NodeID               types.NodeID     `json:"node_id"`
	NodeStatus           types.NodeStatus `json:"node_status"`
	Phase                DrainPhase       `json:"phase"`
	TotalTasks           int              `json:"total_tasks"`
	RemainingTasks       int              `json:"remaining_tasks"`
	ReplacementTasks     int              `json:"replacement_tasks"`
	ReplacementReady     int              `json:"replacement_ready"`
	InsufficientCapacity bool             `json:"insufficient_capacity,omitempty"`
	Message              string           `json:"message,omitempty"`
}

type TaskStatusReport struct {
	TaskID        types.TaskID
	NodeID        types.NodeID
	Status        types.TaskStatus
	ContainerID   string
	FailureReason string
}

type TaskFilter struct {
	ServiceID types.ServiceID
	NodeID    types.NodeID
	Status    types.TaskStatus
}

type RolloutSpec struct {
	Image          string
	MaxUnavailable int
	MaxSurge       int
}

type MemoryService struct {
	mu          sync.RWMutex
	nodes       map[types.NodeID]types.Node
	services    map[types.ServiceID]types.Service
	versions    map[types.ServiceID]map[int64]types.ServiceSpec
	tasks       map[types.TaskID]types.Task
	deployments map[types.DeploymentID]types.Deployment
	secrets     map[string]types.Secret
	registries  map[string]types.RegistryCredential
	envelope    secrets.Envelope
	events      []types.Event
	now         func() time.Time
}

type Option func(*MemoryService)

func WithSecretEnvelope(envelope secrets.Envelope) Option {
	return func(service *MemoryService) {
		if envelope != nil {
			service.envelope = envelope
		}
	}
}

func NewMemoryService(opts ...Option) *MemoryService {
	now := func() time.Time { return time.Now().UTC() }
	envelope, _ := secrets.NewLocalEnvelope("dev-secret-key-change-me")
	service := &MemoryService{
		nodes:       make(map[types.NodeID]types.Node),
		services:    make(map[types.ServiceID]types.Service),
		versions:    make(map[types.ServiceID]map[int64]types.ServiceSpec),
		tasks:       make(map[types.TaskID]types.Task),
		deployments: make(map[types.DeploymentID]types.Deployment),
		secrets:     make(map[string]types.Secret),
		registries:  make(map[string]types.RegistryCredential),
		envelope:    envelope,
		now:         now,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func (s *MemoryService) RegisterNode(ctx context.Context, registration NodeRegistration) (NodeCommand, error) {
	if err := ctx.Err(); err != nil {
		return NodeCommand{}, err
	}
	spec := types.NodeSpec{
		Hostname:         registration.Name,
		AdvertiseAddress: registration.AdvertiseAddress,
		Labels:           registration.Labels,
		Capacity:         registration.Capacity,
		Allocatable:      registration.Allocatable,
	}
	if err := spec.Validate(); err != nil {
		return NodeCommand{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	for id, node := range s.nodes {
		if node.Hostname == registration.Name {
			node.AdvertiseAddress = registration.AdvertiseAddress
			node.Labels = registration.Labels
			node.Capacity = registration.Capacity
			node.Allocatable = registration.Allocatable
			if node.Status == types.NodeUnknown || node.Status == types.NodeOffline {
				if err := types.ValidateNodeTransition(node.Status, types.NodeReady); err != nil {
					return NodeCommand{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
				}
				node.Status = types.NodeReady
			}
			node.LastHeartbeatAt = now
			node.UpdatedAt = now
			s.nodes[id] = node
			s.reconcileAllServicesLocked(now)
			s.appendEventLocked(events.TypeNodeRegistered, types.EventInfo, "controlplane", "node registered", "node", string(id), now)
			return NodeCommand{Node: node, Directives: directivesForNode(node)}, nil
		}
	}

	id := types.NodeID(newUUID())
	node := types.Node{
		ID:               id,
		Hostname:         registration.Name,
		AdvertiseAddress: registration.AdvertiseAddress,
		Labels:           registration.Labels,
		Capacity:         registration.Capacity,
		Allocatable:      registration.Allocatable,
		Status:           types.NodeReady,
		LastHeartbeatAt:  now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	s.nodes[id] = node
	s.reconcileAllServicesLocked(now)
	s.appendEventLocked(events.TypeNodeRegistered, types.EventInfo, "controlplane", "node registered", "node", string(id), now)
	return NodeCommand{Node: node, Directives: directivesForNode(node)}, nil
}

func (s *MemoryService) HeartbeatNode(ctx context.Context, heartbeat NodeHeartbeat) (NodeCommand, error) {
	if err := ctx.Err(); err != nil {
		return NodeCommand{}, err
	}
	if heartbeat.NodeID == "" {
		return NodeCommand{}, fmt.Errorf("%w: node id is required", store.ErrInvalidState)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[heartbeat.NodeID]
	if !ok {
		return NodeCommand{}, store.ErrNotFound
	}
	now := s.now()
	if heartbeat.Capacity.CPU > 0 || heartbeat.Capacity.Memory > 0 {
		node.Capacity = heartbeat.Capacity
	}
	if heartbeat.Allocatable.CPU > 0 || heartbeat.Allocatable.Memory > 0 {
		node.Allocatable = heartbeat.Allocatable
	}
	if heartbeat.Labels != nil {
		node.Labels = heartbeat.Labels
	}
	if heartbeat.Shutdown {
		if err := types.ValidateNodeTransition(node.Status, types.NodeOffline); err != nil {
			return NodeCommand{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
		}
		node.Status = types.NodeOffline
	} else if node.Status == types.NodeOffline {
		if err := types.ValidateNodeTransition(node.Status, types.NodeReady); err != nil {
			return NodeCommand{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
		}
		node.Status = types.NodeReady
	}
	node.LastHeartbeatAt = now
	node.UpdatedAt = now
	s.nodes[node.ID] = node
	if node.Status == types.NodeReady {
		s.reconcileAllServicesLocked(now)
	}

	eventType := events.TypeNodeHeartbeat
	message := "node heartbeat"
	if heartbeat.Shutdown {
		eventType = events.TypeNodeShutdown
		message = "node graceful shutdown"
	}
	s.appendEventLocked(eventType, types.EventInfo, "controlplane", message, "node", string(node.ID), now)
	return NodeCommand{Node: node, Directives: directivesForNode(node)}, nil
}

func (s *MemoryService) MarkStaleNodesOffline(ctx context.Context, timeout time.Duration) ([]types.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("%w: heartbeat timeout must be positive", store.ErrInvalidState)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	cutoff := now.Add(-timeout)
	offline := make([]types.Node, 0)
	for id, node := range s.nodes {
		if node.Status != types.NodeReady && node.Status != types.NodeDraining {
			continue
		}
		if node.LastHeartbeatAt.IsZero() || !node.LastHeartbeatAt.Before(cutoff) {
			continue
		}
		if err := types.ValidateNodeTransition(node.Status, types.NodeOffline); err != nil {
			return nil, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
		}
		node.Status = types.NodeOffline
		node.UpdatedAt = now
		s.nodes[id] = node
		s.markNodeLostTasksLocked(node.ID, now)
		s.appendEventLocked(events.TypeNodeOfflineDetected, types.EventWarning, "controlplane", "node heartbeat timed out", "node", string(node.ID), now)
		offline = append(offline, node)
	}
	if len(offline) > 0 {
		s.reconcileAllServicesLocked(now)
		slices.SortFunc(offline, func(a, b types.Node) int {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
	}
	return offline, nil
}

func (s *MemoryService) SetAgentCredential(ctx context.Context, nodeID types.NodeID, tokenHash string, expiresAt time.Time) (types.Node, error) {
	if err := ctx.Err(); err != nil {
		return types.Node{}, err
	}
	if nodeID == "" {
		return types.Node{}, fmt.Errorf("%w: node id is required", store.ErrInvalidState)
	}
	if tokenHash == "" || expiresAt.IsZero() {
		return types.Node{}, fmt.Errorf("%w: agent credential hash and expiry are required", store.ErrInvalidState)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return types.Node{}, store.ErrNotFound
	}
	node.AgentTokenHash = tokenHash
	node.AgentTokenExpiry = expiresAt.UTC()
	node.AgentRevoked = false
	node.UpdatedAt = s.now()
	s.nodes[nodeID] = node
	return node, nil
}

func (s *MemoryService) RevokeNode(ctx context.Context, nodeID types.NodeID) (types.Node, error) {
	if err := ctx.Err(); err != nil {
		return types.Node{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return types.Node{}, store.ErrNotFound
	}
	node.AgentRevoked = true
	node.UpdatedAt = s.now()
	s.nodes[nodeID] = node
	s.appendEventLocked(events.TypeNodeStatusChanged, types.EventWarning, "controlplane", "node agent credential revoked", "node", string(node.ID), node.UpdatedAt)
	return node, nil
}

func (s *MemoryService) ListAssignedTasks(ctx context.Context, nodeID types.NodeID) ([]AgentTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if nodeID == "" {
		return nil, fmt.Errorf("%w: node id is required", store.ErrInvalidState)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]AgentTask, 0)
	for _, task := range s.tasks {
		if task.NodeID != nodeID {
			continue
		}
		if task.DesiredStatus == types.TaskRemoved || task.ActualStatus == types.TaskRemoved {
			continue
		}
		if task.DesiredStatus == types.TaskRunning && !countsTowardDesiredReplicas(task) {
			continue
		}
		service := s.services[task.ServiceID]
		env, err := s.resolveEnvLocked(ctx, service.Spec)
		if err != nil {
			return nil, err
		}
		auth, err := s.resolveRegistryAuthLocked(ctx, service.Spec.ImagePullSecret)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, AgentTask{
			Task:          task,
			Healthcheck:   service.Spec.Healthcheck,
			Ports:         taskPortsForAgent(task, service),
			Env:           env,
			ImagePullAuth: auth,
		})
	}
	slices.SortFunc(tasks, func(a, b AgentTask) int {
		if a.Task.ID < b.Task.ID {
			return -1
		}
		if a.Task.ID > b.Task.ID {
			return 1
		}
		return 0
	})
	return tasks, nil
}

func taskPortsForAgent(task types.Task, service types.Service) []types.Port {
	if len(task.Ports) > 0 {
		return append([]types.Port(nil), task.Ports...)
	}
	return append([]types.Port(nil), service.Spec.Ports...)
}

func (s *MemoryService) resolveEnvLocked(ctx context.Context, spec types.ServiceSpec) (map[string]string, error) {
	env := cloneStringMap(spec.Env)
	for _, ref := range spec.SecretRefs {
		secret, ok := s.secrets[strings.TrimSpace(ref.Name)]
		if !ok {
			return nil, fmt.Errorf("%w: referenced secret %q not found", store.ErrInvalidState, ref.Name)
		}
		if s.envelope == nil {
			return nil, fmt.Errorf("%w: secret envelope is not configured", store.ErrInvalidState)
		}
		plaintext, err := s.envelope.Decrypt(ctx, secrets.Ciphertext{KeyID: secret.KeyID, Data: secret.EncryptedValue})
		if err != nil {
			return nil, fmt.Errorf("%w: decrypt secret %q", store.ErrInvalidState, ref.Name)
		}
		env[ref.EnvName()] = string(plaintext)
	}
	if len(env) == 0 {
		return nil, nil
	}
	return env, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (s *MemoryService) resolveRegistryAuthLocked(ctx context.Context, id string) (*RegistryAuth, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	credential, ok := s.registries[id]
	if !ok {
		return nil, fmt.Errorf("%w: registry credential %q not found", store.ErrInvalidState, id)
	}
	if s.envelope == nil {
		return nil, fmt.Errorf("%w: secret envelope is not configured", store.ErrInvalidState)
	}
	plaintext, err := s.envelope.Decrypt(ctx, secrets.Ciphertext{KeyID: credential.KeyID, Data: credential.EncryptedPassword})
	if err != nil {
		return nil, fmt.Errorf("%w: decrypt registry credential %q", store.ErrInvalidState, id)
	}
	return &RegistryAuth{
		Username:      credential.Username,
		Password:      string(plaintext),
		ServerAddress: credential.Registry,
	}, nil
}

func (s *MemoryService) ReportTaskStatus(ctx context.Context, report TaskStatusReport) (types.Task, error) {
	if err := ctx.Err(); err != nil {
		return types.Task{}, err
	}
	if report.TaskID == "" {
		return types.Task{}, fmt.Errorf("%w: task id is required", store.ErrInvalidState)
	}
	if report.NodeID == "" {
		return types.Task{}, fmt.Errorf("%w: node id is required", store.ErrInvalidState)
	}
	if !types.ValidAgentTaskStatus(report.Status) {
		return types.Task{}, fmt.Errorf("%w: task status %q is invalid", store.ErrInvalidState, report.Status)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[report.TaskID]
	if !ok {
		return types.Task{}, store.ErrNotFound
	}
	if task.NodeID != report.NodeID {
		return types.Task{}, fmt.Errorf("%w: task is not assigned to node", store.ErrInvalidState)
	}
	if !types.AgentCanReportTaskStatus(task, report.Status) {
		return types.Task{}, fmt.Errorf("%w: task %s with desired=%q actual=%q cannot accept agent status %q", store.ErrInvalidState, task.ID, task.DesiredStatus, task.ActualStatus, report.Status)
	}
	if types.IsTerminalTaskStatus(task.ActualStatus) && task.ActualStatus == report.Status {
		return task, nil
	}
	status := report.Status
	eventType := events.TypeTaskStatus
	severity := types.EventInfo
	message := "task status reported"
	if report.Status == types.TaskFailed {
		eventType = events.TypeTaskFailed
		severity = types.EventError
		message = "task failed"
	}
	if report.Status == types.TaskUnhealthy {
		severity = types.EventWarning
		service := s.services[task.ServiceID]
		if restartAllowed(service.Spec.RestartPolicy) {
			status = types.TaskFailed
			eventType = events.TypeTaskHealthFailed
			message = "task failed healthcheck and needs replacement"
			if report.FailureReason == "" {
				report.FailureReason = "healthcheck unhealthy threshold exceeded"
			}
		} else {
			eventType = events.TypeTaskHealthUnhealthy
			message = "task failed healthcheck"
		}
	}
	task.ActualStatus = status
	task.ContainerID = report.ContainerID
	task.FailureReason = report.FailureReason
	task.UpdatedAt = s.now()
	if (status == types.TaskRunning || status == types.TaskHealthy) && task.StartedAt.IsZero() {
		task.StartedAt = task.UpdatedAt
	}
	if status == types.TaskStopped || status == types.TaskFailed || status == types.TaskRemoved {
		task.FinishedAt = task.UpdatedAt
	}
	s.tasks[task.ID] = task
	if status == types.TaskRemoved {
		s.maybeFinalizeServiceDeletionLocked(task.ServiceID, task.UpdatedAt)
	}
	s.reconcileDrainingNodesLocked(task.UpdatedAt)
	s.appendEventLocked(eventType, severity, "agent", message, "task", string(task.ID), task.UpdatedAt)
	return task, nil
}

func (s *MemoryService) ListNodes(ctx context.Context) ([]types.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]types.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, node)
	}
	slices.SortFunc(nodes, func(a, b types.Node) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return nodes, nil
}

func (s *MemoryService) GetNode(ctx context.Context, id types.NodeID) (types.Node, error) {
	if err := ctx.Err(); err != nil {
		return types.Node{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.nodes[id]
	if !ok {
		return types.Node{}, store.ErrNotFound
	}
	return node, nil
}

func (s *MemoryService) DrainNode(ctx context.Context, id types.NodeID) (types.Node, error) {
	if err := ctx.Err(); err != nil {
		return types.Node{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[id]
	if !ok {
		return types.Node{}, store.ErrNotFound
	}
	if err := types.ValidateNodeTransition(node.Status, types.NodeDraining); err != nil {
		return types.Node{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	now := s.now()
	node.Status = types.NodeDraining
	node.UpdatedAt = now
	s.nodes[id] = node
	s.reconcileNodeDrainLocked(id, now)
	s.appendEventLocked(events.TypeNodeStatusChanged, types.EventInfo, "controlplane", "node status changed", "node", string(id), node.UpdatedAt)
	return node, nil
}

func (s *MemoryService) UncordonNode(ctx context.Context, id types.NodeID) (types.Node, error) {
	return s.setNodeStatus(ctx, id, types.NodeReady)
}

func (s *MemoryService) GetNodeDrainStatus(ctx context.Context, id types.NodeID) (NodeDrainStatus, error) {
	if err := ctx.Err(); err != nil {
		return NodeDrainStatus{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.nodes[id]
	if !ok {
		return NodeDrainStatus{}, store.ErrNotFound
	}
	return s.nodeDrainStatusLocked(node), nil
}

func (s *MemoryService) CreateSecret(ctx context.Context, name string, plaintext string) (types.SecretMetadata, error) {
	if err := ctx.Err(); err != nil {
		return types.SecretMetadata{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return types.SecretMetadata{}, fmt.Errorf("%w: secret name is required", store.ErrInvalidState)
	}
	if s.envelope == nil {
		return types.SecretMetadata{}, fmt.Errorf("%w: secret envelope is not configured", store.ErrInvalidState)
	}
	ciphertext, err := s.envelope.Encrypt(ctx, []byte(plaintext))
	if err != nil {
		return types.SecretMetadata{}, fmt.Errorf("encrypt secret %q: %w", name, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if existing, ok := s.secrets[name]; ok {
		existing.EncryptedValue = append([]byte(nil), ciphertext.Data...)
		existing.KeyID = ciphertext.KeyID
		existing.UpdatedAt = now
		s.secrets[name] = existing
		return existing.Metadata(), nil
	}
	secret := types.Secret{
		Name:           name,
		EncryptedValue: append([]byte(nil), ciphertext.Data...),
		KeyID:          ciphertext.KeyID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.secrets[name] = secret
	s.appendEventLocked(events.TypeSecretCreated, types.EventInfo, "controlplane", "secret created", "secret", "", now)
	return secret.Metadata(), nil
}

func (s *MemoryService) ListSecrets(ctx context.Context) ([]types.SecretMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]types.SecretMetadata, 0, len(s.secrets))
	for _, secret := range s.secrets {
		items = append(items, secret.Metadata())
	}
	slices.SortFunc(items, func(a, b types.SecretMetadata) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return items, nil
}

func (s *MemoryService) GetSecret(ctx context.Context, name string) (types.SecretMetadata, error) {
	if err := ctx.Err(); err != nil {
		return types.SecretMetadata{}, err
	}
	name = strings.TrimSpace(name)
	s.mu.RLock()
	defer s.mu.RUnlock()

	secret, ok := s.secrets[name]
	if !ok {
		return types.SecretMetadata{}, store.ErrNotFound
	}
	return secret.Metadata(), nil
}

func (s *MemoryService) DeleteSecret(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.secrets[name]; !ok {
		return store.ErrNotFound
	}
	delete(s.secrets, name)
	s.appendEventLocked(events.TypeSecretDeleted, types.EventInfo, "controlplane", "secret deleted", "secret", "", s.now())
	return nil
}

func (s *MemoryService) CreateRegistryCredential(ctx context.Context, spec RegistryCredentialSpec) (types.RegistryCredentialMetadata, error) {
	if err := ctx.Err(); err != nil {
		return types.RegistryCredentialMetadata{}, err
	}
	spec.ID = strings.TrimSpace(spec.ID)
	spec.Registry = strings.TrimSpace(spec.Registry)
	spec.Username = strings.TrimSpace(spec.Username)
	if spec.ID == "" {
		return types.RegistryCredentialMetadata{}, fmt.Errorf("%w: registry credential id is required", store.ErrInvalidState)
	}
	if spec.Registry == "" {
		return types.RegistryCredentialMetadata{}, fmt.Errorf("%w: registry host is required", store.ErrInvalidState)
	}
	if spec.Username == "" {
		return types.RegistryCredentialMetadata{}, fmt.Errorf("%w: registry username is required", store.ErrInvalidState)
	}
	if s.envelope == nil {
		return types.RegistryCredentialMetadata{}, fmt.Errorf("%w: secret envelope is not configured", store.ErrInvalidState)
	}
	ciphertext, err := s.envelope.Encrypt(ctx, []byte(spec.Password))
	if err != nil {
		return types.RegistryCredentialMetadata{}, fmt.Errorf("encrypt registry credential %q: %w", spec.ID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if existing, ok := s.registries[spec.ID]; ok {
		existing.Registry = spec.Registry
		existing.Username = spec.Username
		existing.EncryptedPassword = append([]byte(nil), ciphertext.Data...)
		existing.KeyID = ciphertext.KeyID
		existing.UpdatedAt = now
		s.registries[spec.ID] = existing
		return existing.Metadata(), nil
	}
	credential := types.RegistryCredential{
		ID:                spec.ID,
		Registry:          spec.Registry,
		Username:          spec.Username,
		EncryptedPassword: append([]byte(nil), ciphertext.Data...),
		KeyID:             ciphertext.KeyID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.registries[spec.ID] = credential
	s.appendEventLocked(events.TypeRegistryCredCreated, types.EventInfo, "controlplane", "registry credential created", "registry_credential", "", now)
	return credential.Metadata(), nil
}

func (s *MemoryService) ListRegistryCredentials(ctx context.Context) ([]types.RegistryCredentialMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]types.RegistryCredentialMetadata, 0, len(s.registries))
	for _, credential := range s.registries {
		items = append(items, credential.Metadata())
	}
	slices.SortFunc(items, func(a, b types.RegistryCredentialMetadata) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return items, nil
}

func (s *MemoryService) DeleteRegistryCredential(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.registries[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.registries, id)
	s.appendEventLocked(events.TypeRegistryCredDeleted, types.EventInfo, "controlplane", "registry credential deleted", "registry_credential", "", s.now())
	return nil
}

func (s *MemoryService) CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error) {
	if err := ctx.Err(); err != nil {
		return types.Service{}, err
	}
	normalized, err := types.NormalizeServiceSpec(spec, types.DefaultResourceDefaults())
	if err != nil {
		return types.Service{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	spec = normalized
	if err := spec.Validate(); err != nil {
		return types.Service{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, service := range s.services {
		if service.Spec.Name == spec.Name {
			return types.Service{}, store.ErrDuplicate
		}
	}
	for _, ref := range spec.SecretRefs {
		if _, ok := s.secrets[strings.TrimSpace(ref.Name)]; !ok {
			return types.Service{}, fmt.Errorf("%w: referenced secret %q not found", store.ErrInvalidState, ref.Name)
		}
	}
	if imagePullSecret := strings.TrimSpace(spec.ImagePullSecret); imagePullSecret != "" {
		if _, ok := s.registries[imagePullSecret]; !ok {
			return types.Service{}, fmt.Errorf("%w: registry credential %q not found", store.ErrInvalidState, imagePullSecret)
		}
		spec.ImagePullSecret = imagePullSecret
	}

	now := s.now()
	service := types.Service{
		ID:                types.ServiceID(newUUID()),
		Spec:              spec,
		Status:            types.ServiceActive,
		DeploymentVersion: 1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.services[service.ID] = service
	s.storeServiceVersionLocked(service.ID, service.DeploymentVersion, spec)
	s.reconcileServiceTasksLocked(service, now)
	s.appendEventLocked(events.TypeServiceCreated, types.EventInfo, "controlplane", "service created", "service", string(service.ID), now)
	return service, nil
}

func (s *MemoryService) ListServices(ctx context.Context) ([]types.Service, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	services := make([]types.Service, 0, len(s.services))
	for _, service := range s.services {
		services = append(services, service)
	}
	slices.SortFunc(services, func(a, b types.Service) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return services, nil
}

func (s *MemoryService) GetService(ctx context.Context, id types.ServiceID) (types.Service, error) {
	if err := ctx.Err(); err != nil {
		return types.Service{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	service, ok := s.services[id]
	if !ok {
		return types.Service{}, store.ErrNotFound
	}
	return service, nil
}

func (s *MemoryService) DeleteService(ctx context.Context, id types.ServiceID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	service, ok := s.services[id]
	if !ok {
		return store.ErrNotFound
	}
	if service.Status == types.ServiceDeleted || service.Status == types.ServiceDeleting {
		return nil
	}
	if err := types.ValidateServiceTransition(service.Status, types.ServiceDeleting); err != nil {
		return fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	now := s.now()
	s.cancelActiveDeploymentsLocked(id, now, "service deletion requested")
	service.Status = types.ServiceDeleting
	service.UpdatedAt = now
	s.services[id] = service
	for taskID, task := range s.tasks {
		if task.ServiceID == id {
			if task.DesiredStatus != types.TaskRemoved && task.ActualStatus != types.TaskRemoved {
				if err := types.ValidateTaskDesiredTransition(task.DesiredStatus, types.TaskStopped); err != nil {
					return fmt.Errorf("%w: %v", store.ErrInvalidState, err)
				}
				task.DesiredStatus = types.TaskStopped
				task.UpdatedAt = now
				s.tasks[taskID] = task
			}
		}
	}
	s.appendEventLocked(events.TypeServiceDeletionStarted, types.EventInfo, "controlplane", "service deletion requested", "service", string(id), now)
	s.maybeFinalizeServiceDeletionLocked(id, now)
	return nil
}

func (s *MemoryService) ScaleService(ctx context.Context, id types.ServiceID, replicas int) (types.Service, error) {
	if err := ctx.Err(); err != nil {
		return types.Service{}, err
	}
	if replicas < 0 {
		return types.Service{}, fmt.Errorf("%w: replicas cannot be negative", store.ErrInvalidState)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	service, ok := s.services[id]
	if !ok {
		return types.Service{}, store.ErrNotFound
	}
	if service.Status != "" && service.Status != types.ServiceActive {
		return types.Service{}, operationConflict("delete")
	}
	if active := s.activeDeploymentLocked(id); active.ID != "" {
		return types.Service{}, operationConflict(string(operationForDeployment(active)))
	}
	service.Spec.Replicas = replicas
	service.UpdatedAt = s.now()
	s.services[id] = service
	s.reconcileServiceTasksLocked(service, service.UpdatedAt)
	s.appendEventLocked(events.TypeServiceScaled, types.EventInfo, "controlplane", "service scaled", "service", string(id), service.UpdatedAt)
	return service, nil
}

func (s *MemoryService) RolloutService(ctx context.Context, id types.ServiceID, spec RolloutSpec) (types.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return types.Deployment{}, err
	}
	spec.Image = strings.TrimSpace(spec.Image)
	if spec.Image == "" {
		return types.Deployment{}, fmt.Errorf("%w: image is required", store.ErrInvalidState)
	}
	if spec.MaxUnavailable < 0 {
		return types.Deployment{}, fmt.Errorf("%w: maxUnavailable cannot be negative", store.ErrInvalidState)
	}
	if spec.MaxSurge < 0 {
		return types.Deployment{}, fmt.Errorf("%w: maxSurge cannot be negative", store.ErrInvalidState)
	}
	if spec.MaxUnavailable == 0 && spec.MaxSurge == 0 {
		return types.Deployment{}, fmt.Errorf("%w: maxUnavailable and maxSurge cannot both be zero", store.ErrInvalidState)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	service, ok := s.services[id]
	if !ok {
		return types.Deployment{}, store.ErrNotFound
	}
	if service.Status != "" && service.Status != types.ServiceActive {
		return types.Deployment{}, operationConflict("delete")
	}
	if active := s.activeDeploymentLocked(id); active.ID != "" {
		if operationForDeployment(active) == serviceOperationRollout &&
			service.Spec.Image == spec.Image &&
			active.MaxUnavailable == spec.MaxUnavailable &&
			active.MaxSurge == spec.MaxSurge {
			return active, nil
		}
		return types.Deployment{}, operationConflict(string(operationForDeployment(active)))
	}
	now := s.now()
	fromVersion := service.DeploymentVersion
	service.Spec.Image = spec.Image
	service.DeploymentVersion++
	service.UpdatedAt = now
	s.services[id] = service
	s.storeServiceVersionLocked(id, service.DeploymentVersion, service.Spec)

	deployment := types.Deployment{
		ID:             types.DeploymentID(newUUID()),
		ServiceID:      id,
		FromVersion:    fromVersion,
		ToVersion:      service.DeploymentVersion,
		Strategy:       types.RolloutRollingUpdate,
		Status:         types.DeploymentPending,
		MaxUnavailable: spec.MaxUnavailable,
		MaxSurge:       spec.MaxSurge,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.deployments[deployment.ID] = deployment
	s.appendEventLocked(events.TypeRolloutStarted, types.EventInfo, "controlplane", "service rollout started", "service", string(id), now)
	return deployment, nil
}

func (s *MemoryService) GetDeployment(ctx context.Context, id types.DeploymentID) (types.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return types.Deployment{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	deployment, ok := s.deployments[id]
	if !ok {
		return types.Deployment{}, store.ErrNotFound
	}
	return deployment, nil
}

func (s *MemoryService) GetServiceRollout(ctx context.Context, id types.ServiceID) (types.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return types.Deployment{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.services[id]; !ok {
		return types.Deployment{}, store.ErrNotFound
	}
	var latest types.Deployment
	for _, deployment := range s.deployments {
		if deployment.ServiceID != id {
			continue
		}
		if latest.ID == "" || deployment.CreatedAt.After(latest.CreatedAt) || (deployment.CreatedAt.Equal(latest.CreatedAt) && deployment.ID > latest.ID) {
			latest = deployment
		}
	}
	if latest.ID == "" {
		return types.Deployment{}, store.ErrNotFound
	}
	return latest, nil
}

func (s *MemoryService) RollbackService(ctx context.Context, id types.ServiceID) (types.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return types.Deployment{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	service, ok := s.services[id]
	if !ok {
		return types.Deployment{}, store.ErrNotFound
	}
	if service.Status != "" && service.Status != types.ServiceActive {
		return types.Deployment{}, operationConflict("delete")
	}
	if active := s.activeRollbackLocked(id); active.ID != "" {
		return active, nil
	}
	if active := s.activeDeploymentLocked(id); active.ID != "" {
		return types.Deployment{}, operationConflict(string(operationForDeployment(active)))
	}
	targetVersion, targetSpec, ok := s.previousSuccessfulVersionLocked(service)
	if !ok {
		return types.Deployment{}, fmt.Errorf("%w: no previous successful deployment version", store.ErrInvalidState)
	}

	now := s.now()
	fromVersion := service.DeploymentVersion
	service.Spec = targetSpec
	service.DeploymentVersion = targetVersion
	service.UpdatedAt = now
	s.services[id] = service

	deployment := types.Deployment{
		ID:             types.DeploymentID(newUUID()),
		ServiceID:      id,
		FromVersion:    fromVersion,
		ToVersion:      targetVersion,
		Strategy:       types.RolloutRollingUpdate,
		Status:         types.DeploymentRollingBack,
		MaxUnavailable: 1,
		MaxSurge:       0,
		CreatedAt:      now,
		UpdatedAt:      now,
		StartedAt:      now,
	}
	s.deployments[deployment.ID] = deployment
	s.appendEventLocked(events.TypeRollbackStarted, types.EventInfo, "controlplane", "service rollback started", "service", string(id), now)
	return deployment, nil
}

func (s *MemoryService) ListTasks(ctx context.Context, filter TaskFilter) ([]types.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]types.Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		if filter.ServiceID != "" && task.ServiceID != filter.ServiceID {
			continue
		}
		if filter.NodeID != "" && task.NodeID != filter.NodeID {
			continue
		}
		if filter.Status != "" && task.ActualStatus != filter.Status {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (s *MemoryService) GetTask(ctx context.Context, id types.TaskID) (types.Task, error) {
	if err := ctx.Err(); err != nil {
		return types.Task{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return types.Task{}, store.ErrNotFound
	}
	return task, nil
}

func (s *MemoryService) CreateTask(ctx context.Context, task types.Task) (types.Task, error) {
	if err := ctx.Err(); err != nil {
		return types.Task{}, err
	}
	if task.ServiceID == "" {
		return types.Task{}, fmt.Errorf("%w: service id is required", store.ErrInvalidState)
	}
	if task.DesiredStatus == "" {
		task.DesiredStatus = types.TaskRunning
	}
	if task.ActualStatus == "" {
		task.ActualStatus = types.TaskPending
	}
	if !types.ValidTaskStatus(task.DesiredStatus) || !types.ValidTaskStatus(task.ActualStatus) {
		return types.Task{}, fmt.Errorf("%w: task status is invalid", store.ErrInvalidState)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.services[task.ServiceID]; !ok {
		return types.Task{}, store.ErrNotFound
	}
	now := s.now()
	if task.ID == "" {
		task.ID = types.TaskID(newUUID())
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	s.tasks[task.ID] = task
	return task, nil
}

func (s *MemoryService) StopTask(ctx context.Context, id types.TaskID, expectedUpdatedAt time.Time) (types.Task, error) {
	if err := ctx.Err(); err != nil {
		return types.Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return types.Task{}, store.ErrNotFound
	}
	if !expectedUpdatedAt.IsZero() && !task.UpdatedAt.Equal(expectedUpdatedAt) {
		return types.Task{}, store.ErrConflict
	}
	if task.DesiredStatus == types.TaskStopped || task.DesiredStatus == types.TaskRemoved {
		return task, nil
	}
	if err := types.ValidateTaskDesiredTransition(task.DesiredStatus, types.TaskStopped); err != nil {
		return types.Task{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	task.DesiredStatus = types.TaskStopped
	task.UpdatedAt = s.now()
	s.tasks[id] = task
	return task, nil
}

func (s *MemoryService) ListTasksByService(ctx context.Context, serviceID types.ServiceID) ([]types.Task, error) {
	return s.ListTasks(ctx, TaskFilter{ServiceID: serviceID})
}

func (s *MemoryService) UpdateServiceStatus(ctx context.Context, id types.ServiceID, status types.ServiceStatus, expectedUpdatedAt time.Time) (types.Service, error) {
	if err := ctx.Err(); err != nil {
		return types.Service{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	service, ok := s.services[id]
	if !ok {
		return types.Service{}, store.ErrNotFound
	}
	if !expectedUpdatedAt.IsZero() && !service.UpdatedAt.Equal(expectedUpdatedAt) {
		return types.Service{}, store.ErrConflict
	}
	if err := types.ValidateServiceTransition(service.Status, status); err != nil {
		return types.Service{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	service.Status = status
	service.UpdatedAt = s.now()
	s.services[id] = service
	if status == types.ServiceDeleted {
		s.appendEventLocked(events.TypeServiceDeleted, types.EventInfo, "controlplane", "service deleted", "service", string(id), service.UpdatedAt)
	}
	return service, nil
}

func (s *MemoryService) ListDeploymentsByStatus(ctx context.Context, status types.DeploymentStatus) ([]types.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	deployments := make([]types.Deployment, 0)
	for _, deployment := range s.deployments {
		if deployment.Status == status {
			deployments = append(deployments, deployment)
		}
	}
	slices.SortFunc(deployments, func(a, b types.Deployment) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return deployments, nil
}

func (s *MemoryService) UpdateDeploymentStatus(ctx context.Context, id types.DeploymentID, status types.DeploymentStatus, expectedUpdatedAt time.Time) (types.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return types.Deployment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	deployment, ok := s.deployments[id]
	if !ok {
		return types.Deployment{}, store.ErrNotFound
	}
	if !expectedUpdatedAt.IsZero() && !deployment.UpdatedAt.Equal(expectedUpdatedAt) {
		return types.Deployment{}, store.ErrConflict
	}
	if err := types.ValidateDeploymentTransition(deployment.Status, status); err != nil {
		return types.Deployment{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	now := s.now()
	deployment.Status = status
	deployment.UpdatedAt = now
	if (status == types.DeploymentRunning || status == types.DeploymentRollingBack) && deployment.StartedAt.IsZero() {
		deployment.StartedAt = now
	}
	if status == types.DeploymentSucceeded || status == types.DeploymentFailed || status == types.DeploymentPaused || status == types.DeploymentRolledBack {
		if deployment.CompletedAt.IsZero() {
			deployment.CompletedAt = now
		}
	}
	s.deployments[id] = deployment
	return deployment, nil
}

func (s *MemoryService) AppendEvent(ctx context.Context, event types.Event) (types.Event, error) {
	if err := ctx.Err(); err != nil {
		return types.Event{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.ID == "" {
		event.ID = types.EventID(newUUID())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = s.now()
	}
	event.Timestamp = event.Timestamp.UTC()
	if event.Severity == "" {
		event.Severity = types.EventInfo
	}
	s.events = append(s.events, event)
	return event, nil
}

func (s *MemoryService) ListEvents(ctx context.Context, filter events.Filter) ([]types.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	events := make([]types.Event, 0, len(s.events))
	for i := len(s.events) - 1; i >= 0; i-- {
		event := s.events[i]
		if filter.ServiceID != "" && (event.RelatedObjectType != "service" || event.RelatedObjectID != string(filter.ServiceID)) {
			continue
		}
		if filter.TaskID != "" && (event.RelatedObjectType != "task" || event.RelatedObjectID != string(filter.TaskID)) {
			continue
		}
		if filter.NodeID != "" && (event.RelatedObjectType != "node" || event.RelatedObjectID != string(filter.NodeID)) {
			continue
		}
		if filter.Type != "" && event.Type != filter.Type {
			continue
		}
		if filter.Severity != "" && event.Severity != filter.Severity {
			continue
		}
		if !filter.Since.IsZero() && event.Timestamp.Before(filter.Since) {
			continue
		}
		events = append(events, event)
		if len(events) == limit {
			break
		}
	}
	return events, nil
}

func (s *MemoryService) setNodeStatus(ctx context.Context, id types.NodeID, status types.NodeStatus) (types.Node, error) {
	if err := ctx.Err(); err != nil {
		return types.Node{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[id]
	if !ok {
		return types.Node{}, store.ErrNotFound
	}
	if err := types.ValidateNodeTransition(node.Status, status); err != nil {
		return types.Node{}, fmt.Errorf("%w: %v", store.ErrInvalidState, err)
	}
	node.Status = status
	node.UpdatedAt = s.now()
	s.nodes[id] = node
	if status == types.NodeReady {
		s.reconcileAllServicesLocked(node.UpdatedAt)
	}
	s.appendEventLocked(events.TypeNodeStatusChanged, types.EventInfo, "controlplane", "node status changed", "node", string(id), node.UpdatedAt)
	return node, nil
}

func (s *MemoryService) markNodeLostTasksLocked(nodeID types.NodeID, timestamp time.Time) {
	for taskID, task := range s.tasks {
		if task.NodeID != nodeID || !types.IsActiveTask(task) {
			continue
		}
		task.Conditions = upsertTaskCondition(task.Conditions, types.TaskCondition{
			Type:               types.TaskConditionNodeLost,
			Message:            "node heartbeat timed out",
			LastTransitionTime: timestamp,
		})
		service := s.services[task.ServiceID]
		if !service.Spec.Stateful {
			task.DesiredStatus = types.TaskRemoved
			task.ActualStatus = types.TaskFailed
			task.FailureReason = "node_lost"
			task.FinishedAt = timestamp
		}
		task.UpdatedAt = timestamp
		s.tasks[taskID] = task
	}
}

func upsertTaskCondition(conditions []types.TaskCondition, condition types.TaskCondition) []types.TaskCondition {
	condition.LastTransitionTime = condition.LastTransitionTime.UTC()
	for i, existing := range conditions {
		if existing.Type == condition.Type {
			if existing.Message == condition.Message {
				return conditions
			}
			conditions[i] = condition
			return conditions
		}
	}
	return append(conditions, condition)
}

func (s *MemoryService) appendEventLocked(eventType string, severity types.EventSeverity, source string, message string, objectType string, objectID string, timestamp time.Time) {
	s.events = append(s.events, types.Event{
		ID:                types.EventID(newUUID()),
		Type:              eventType,
		Severity:          severity,
		Source:            source,
		Message:           message,
		RelatedObjectType: objectType,
		RelatedObjectID:   objectID,
		Timestamp:         timestamp.UTC(),
	})
}

func directivesForNode(node types.Node) []AgentDirective {
	switch node.Status {
	case types.NodeDraining:
		return []AgentDirective{{Type: "drain", Message: "node is marked draining"}}
	case types.NodeOffline:
		return []AgentDirective{{Type: "offline", Message: "node is marked offline"}}
	default:
		return nil
	}
}

func (s *MemoryService) reconcileAllServicesLocked(timestamp time.Time) {
	services := make([]types.Service, 0, len(s.services))
	for _, service := range s.services {
		services = append(services, service)
	}
	slices.SortFunc(services, func(a, b types.Service) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	for _, service := range services {
		s.reconcileServiceTasksLocked(service, timestamp)
	}
}

func (s *MemoryService) reconcileServiceTasksLocked(service types.Service, timestamp time.Time) {
	if service.Status != "" && service.Status != types.ServiceActive {
		return
	}
	if s.hasActiveDeploymentLocked(service.ID) {
		return
	}
	active := make([]types.Task, 0)
	for _, task := range s.tasks {
		if task.ServiceID != service.ID {
			continue
		}
		if !countsTowardDesiredReplicas(task) {
			continue
		}
		if task.Image != service.Spec.Image || task.Version != service.DeploymentVersion {
			if err := types.ValidateTaskDesiredTransition(task.DesiredStatus, types.TaskRemoved); err != nil {
				continue
			}
			task.DesiredStatus = types.TaskRemoved
			task.UpdatedAt = timestamp
			s.tasks[task.ID] = task
			continue
		}
		active = append(active, task)
	}
	slices.SortFunc(active, func(a, b types.Task) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	nodes := s.readyNodesLocked()
	if len(nodes) == 0 {
		return
	}

	for len(active) < service.Spec.Replicas {
		node := nodes[len(active)%len(nodes)]
		task := types.Task{
			ID:            types.TaskID(newUUID()),
			ServiceID:     service.ID,
			NodeID:        node.ID,
			DesiredStatus: types.TaskRunning,
			ActualStatus:  types.TaskAssigned,
			Image:         service.Spec.Image,
			Version:       service.DeploymentVersion,
			Ports:         s.assignPortsForNodeLocked(service.Spec.Ports, node.ID),
			CreatedAt:     timestamp,
			UpdatedAt:     timestamp,
		}
		s.tasks[task.ID] = task
		active = append(active, task)
	}

	for i, task := range active {
		if i >= service.Spec.Replicas {
			if err := types.ValidateTaskDesiredTransition(task.DesiredStatus, types.TaskRemoved); err != nil {
				continue
			}
			task.DesiredStatus = types.TaskRemoved
			task.UpdatedAt = timestamp
			s.tasks[task.ID] = task
			continue
		}
	}
}

func (s *MemoryService) reconcileDrainingNodesLocked(timestamp time.Time) {
	nodes := make([]types.NodeID, 0)
	for _, node := range s.nodes {
		if node.Status == types.NodeDraining {
			nodes = append(nodes, node.ID)
		}
	}
	slices.Sort(nodes)
	for _, nodeID := range nodes {
		s.reconcileNodeDrainLocked(nodeID, timestamp)
	}
}

func (s *MemoryService) reconcileNodeDrainLocked(nodeID types.NodeID, timestamp time.Time) {
	node, ok := s.nodes[nodeID]
	if !ok || node.Status != types.NodeDraining {
		return
	}
	readyNodes := s.readyNodesLocked()
	if len(readyNodes) == 0 {
		if s.nodeDrainStatusLocked(node).RemainingTasks > 0 {
			s.appendEventLocked(events.TypeNodeDrainPending, types.EventWarning, "controlplane", "node drain is waiting for ready replacement capacity", "node", string(nodeID), timestamp)
		}
		return
	}
	services := make([]types.Service, 0, len(s.services))
	for _, service := range s.services {
		if service.Status != "" && service.Status != types.ServiceActive {
			continue
		}
		services = append(services, service)
	}
	slices.SortFunc(services, func(a, b types.Service) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	for _, service := range services {
		drainingTasks := s.activeServiceTasksOnNodeLocked(service.ID, nodeID)
		if len(drainingTasks) == 0 {
			continue
		}
		offNodeActive := s.activeServiceTasksOffNodeLocked(service.ID, nodeID)
		for len(offNodeActive) < service.Spec.Replicas {
			node := readyNodes[len(offNodeActive)%len(readyNodes)]
			task := types.Task{
				ID:            types.TaskID(newUUID()),
				ServiceID:     service.ID,
				NodeID:        node.ID,
				DesiredStatus: types.TaskRunning,
				ActualStatus:  types.TaskAssigned,
				Image:         service.Spec.Image,
				Version:       service.DeploymentVersion,
				Ports:         s.assignPortsForNodeLocked(service.Spec.Ports, node.ID),
				CreatedAt:     timestamp,
				UpdatedAt:     timestamp,
			}
			s.tasks[task.ID] = task
			offNodeActive = append(offNodeActive, task)
			s.appendEventLocked(events.TypeTaskAssigned, types.EventInfo, "controlplane", "replacement task assigned during node drain", "task", string(task.ID), timestamp)
		}
		if availableTaskCount(offNodeActive) < service.Spec.Replicas {
			continue
		}
		for _, task := range drainingTasks {
			if task.DesiredStatus == types.TaskStopped || task.DesiredStatus == types.TaskRemoved {
				continue
			}
			if err := types.ValidateTaskDesiredTransition(task.DesiredStatus, types.TaskStopped); err != nil {
				continue
			}
			task.DesiredStatus = types.TaskStopped
			task.UpdatedAt = timestamp
			s.tasks[task.ID] = task
		}
	}
}

func (s *MemoryService) activeServiceTasksOnNodeLocked(serviceID types.ServiceID, nodeID types.NodeID) []types.Task {
	tasks := make([]types.Task, 0)
	for _, task := range s.tasks {
		if task.ServiceID != serviceID || task.NodeID != nodeID || !types.IsActiveTask(task) {
			continue
		}
		tasks = append(tasks, task)
	}
	sortTasksByID(tasks)
	return tasks
}

func (s *MemoryService) activeServiceTasksOffNodeLocked(serviceID types.ServiceID, nodeID types.NodeID) []types.Task {
	tasks := make([]types.Task, 0)
	for _, task := range s.tasks {
		if task.ServiceID != serviceID || task.NodeID == nodeID || !types.IsActiveTask(task) {
			continue
		}
		tasks = append(tasks, task)
	}
	sortTasksByID(tasks)
	return tasks
}

func sortTasksByID(tasks []types.Task) {
	slices.SortFunc(tasks, func(a, b types.Task) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
}

func availableTaskCount(tasks []types.Task) int {
	count := 0
	for _, task := range tasks {
		if types.IsAvailableTaskStatus(task.ActualStatus) {
			count++
		}
	}
	return count
}

func (s *MemoryService) nodeDrainStatusLocked(node types.Node) NodeDrainStatus {
	status := NodeDrainStatus{
		NodeID:     node.ID,
		NodeStatus: node.Status,
	}
	if node.Status == types.NodeOffline {
		status.Phase = DrainOffline
		status.Message = "node is offline during drain"
		return status
	}
	if node.Status != types.NodeDraining {
		status.Phase = DrainNotDraining
		status.Message = "node is not draining"
		return status
	}
	for _, task := range s.tasks {
		if task.NodeID != node.ID || !types.IsActiveTask(task) {
			continue
		}
		status.TotalTasks++
		if task.DesiredStatus != types.TaskStopped && task.DesiredStatus != types.TaskRemoved {
			status.RemainingTasks++
		}
		service := s.services[task.ServiceID]
		offNode := s.activeServiceTasksOffNodeLocked(service.ID, node.ID)
		status.ReplacementTasks += len(offNode)
		status.ReplacementReady += availableTaskCount(offNode)
	}
	if status.RemainingTasks == 0 {
		status.Phase = DrainComplete
		status.Message = "node drain completed"
		return status
	}
	status.Phase = DrainPending
	if len(s.readyNodesLocked()) == 0 {
		status.InsufficientCapacity = true
		status.Message = "waiting for ready replacement capacity"
	} else {
		status.Message = "waiting for replacement tasks to become healthy"
	}
	return status
}

func countsTowardDesiredReplicas(task types.Task) bool {
	return types.IsActiveTask(task)
}

const (
	memoryDynamicPortStart = 30000
	memoryDynamicPortEnd   = 32767
)

func (s *MemoryService) assignPortsForNodeLocked(ports []types.Port, nodeID types.NodeID) []types.Port {
	if len(ports) == 0 {
		return nil
	}
	assigned := make([]types.Port, 0, len(ports))
	reserved := s.reservedPortsForNodeLocked(nodeID)
	for _, port := range ports {
		candidate := port
		if candidate.Protocol == "" {
			candidate.Protocol = types.PortTCP
		}
		if candidate.PublishedPort <= 0 {
			candidate.PublishedPort = firstFreeMemoryPort(candidate.Protocol, reserved)
		}
		if candidate.PublishedPort > 0 {
			reserved[memoryPortKey{protocol: candidate.Protocol, port: candidate.PublishedPort}] = struct{}{}
		}
		assigned = append(assigned, candidate)
	}
	return assigned
}

func (s *MemoryService) reservedPortsForNodeLocked(nodeID types.NodeID) map[memoryPortKey]struct{} {
	reserved := make(map[memoryPortKey]struct{})
	for _, task := range s.tasks {
		if task.NodeID != nodeID || !types.IsActiveTask(task) {
			continue
		}
		service := s.services[task.ServiceID]
		for _, port := range taskPortsForAgent(task, service) {
			if port.PublishedPort <= 0 {
				continue
			}
			reserved[memoryPortKey{protocol: port.Protocol, port: port.PublishedPort}] = struct{}{}
		}
	}
	return reserved
}

func firstFreeMemoryPort(protocol types.PortProtocol, reserved map[memoryPortKey]struct{}) int {
	for port := memoryDynamicPortStart; port <= memoryDynamicPortEnd; port++ {
		if _, ok := reserved[memoryPortKey{protocol: protocol, port: port}]; !ok {
			return port
		}
	}
	return 0
}

type memoryPortKey struct {
	protocol types.PortProtocol
	port     int
}

func (s *MemoryService) hasActiveDeploymentLocked(serviceID types.ServiceID) bool {
	return s.activeDeploymentLocked(serviceID).ID != ""
}

func (s *MemoryService) activeDeploymentLocked(serviceID types.ServiceID) types.Deployment {
	var latest types.Deployment
	for _, deployment := range s.deployments {
		if deployment.ServiceID != serviceID || !isActiveDeploymentStatus(deployment.Status) {
			continue
		}
		if latest.ID == "" || deployment.CreatedAt.After(latest.CreatedAt) || (deployment.CreatedAt.Equal(latest.CreatedAt) && deployment.ID > latest.ID) {
			latest = deployment
		}
	}
	return latest
}

func (s *MemoryService) cancelActiveDeploymentsLocked(serviceID types.ServiceID, timestamp time.Time, reason string) {
	deploymentIDs := make([]types.DeploymentID, 0)
	for _, deployment := range s.deployments {
		if deployment.ServiceID != serviceID || !isActiveDeploymentStatus(deployment.Status) {
			continue
		}
		deploymentIDs = append(deploymentIDs, deployment.ID)
	}
	slices.Sort(deploymentIDs)
	for _, id := range deploymentIDs {
		deployment := s.deployments[id]
		deployment.Status = types.DeploymentFailed
		deployment.UpdatedAt = timestamp
		deployment.CompletedAt = timestamp
		s.deployments[id] = deployment
		s.appendEventLocked(events.TypeRolloutStatusChanged, types.EventWarning, "controlplane", reason, "service", string(serviceID), timestamp)
	}
}

func isActiveDeploymentStatus(status types.DeploymentStatus) bool {
	return status == types.DeploymentPending || status == types.DeploymentRunning || status == types.DeploymentRollingBack
}

func (s *MemoryService) maybeFinalizeServiceDeletionLocked(serviceID types.ServiceID, timestamp time.Time) {
	service, ok := s.services[serviceID]
	if !ok || service.Status != types.ServiceDeleting {
		return
	}
	for _, task := range s.tasks {
		if task.ServiceID != serviceID {
			continue
		}
		if task.ActualStatus != types.TaskRemoved {
			return
		}
	}
	if err := types.ValidateServiceTransition(service.Status, types.ServiceDeleted); err != nil {
		return
	}
	service.Status = types.ServiceDeleted
	service.UpdatedAt = timestamp
	s.services[serviceID] = service
	s.appendEventLocked(events.TypeServiceDeleted, types.EventInfo, "controlplane", "service deleted", "service", string(serviceID), timestamp)
}

func (s *MemoryService) activeRollbackLocked(serviceID types.ServiceID) types.Deployment {
	var latest types.Deployment
	for _, deployment := range s.deployments {
		if deployment.ServiceID != serviceID || deployment.ToVersion >= deployment.FromVersion {
			continue
		}
		if deployment.Status != types.DeploymentRollingBack && deployment.Status != types.DeploymentPending && deployment.Status != types.DeploymentRunning {
			continue
		}
		if latest.ID == "" || deployment.CreatedAt.After(latest.CreatedAt) || (deployment.CreatedAt.Equal(latest.CreatedAt) && deployment.ID > latest.ID) {
			latest = deployment
		}
	}
	return latest
}

type serviceOperation string

const (
	serviceOperationRollout  serviceOperation = "rollout"
	serviceOperationRollback serviceOperation = "rollback"
)

func operationForDeployment(deployment types.Deployment) serviceOperation {
	if deployment.ToVersion < deployment.FromVersion || deployment.Status == types.DeploymentRollingBack {
		return serviceOperationRollback
	}
	return serviceOperationRollout
}

func operationConflict(operation string) error {
	return fmt.Errorf("%w: operation already in progress: %s", store.ErrConflict, operation)
}

func (s *MemoryService) previousSuccessfulVersionLocked(service types.Service) (int64, types.ServiceSpec, bool) {
	version := int64(0)
	if _, ok := s.versions[service.ID][1]; ok && service.DeploymentVersion > 1 {
		version = 1
	}
	for _, deployment := range s.deployments {
		if deployment.ServiceID != service.ID {
			continue
		}
		if deployment.ToVersion >= service.DeploymentVersion {
			continue
		}
		if deployment.Status != types.DeploymentSucceeded && deployment.Status != types.DeploymentRolledBack {
			continue
		}
		if deployment.ToVersion > version {
			version = deployment.ToVersion
		}
	}
	if version == 0 {
		return 0, types.ServiceSpec{}, false
	}
	spec, ok := s.versions[service.ID][version]
	return version, spec, ok
}

func (s *MemoryService) storeServiceVersionLocked(serviceID types.ServiceID, version int64, spec types.ServiceSpec) {
	if s.versions[serviceID] == nil {
		s.versions[serviceID] = make(map[int64]types.ServiceSpec)
	}
	s.versions[serviceID][version] = spec
}

func (s *MemoryService) readyNodesLocked() []types.Node {
	nodes := make([]types.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		if node.Status == types.NodeReady {
			nodes = append(nodes, node)
		}
	}
	slices.SortFunc(nodes, func(a, b types.Node) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return nodes
}

func restartAllowed(policy types.RestartPolicy) bool {
	switch policy.Condition {
	case "", types.RestartAlways, types.RestartOnFailure:
		return true
	default:
		return false
	}
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(errors.New("generate uuid: " + err.Error()))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
