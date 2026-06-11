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
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

type Service interface {
	RegisterNode(ctx context.Context, registration NodeRegistration) (NodeCommand, error)
	HeartbeatNode(ctx context.Context, heartbeat NodeHeartbeat) (NodeCommand, error)
	ListAssignedTasks(ctx context.Context, nodeID types.NodeID) ([]AgentTask, error)
	ReportTaskStatus(ctx context.Context, report TaskStatusReport) (types.Task, error)
	ListNodes(ctx context.Context) ([]types.Node, error)
	GetNode(ctx context.Context, id types.NodeID) (types.Node, error)
	DrainNode(ctx context.Context, id types.NodeID) (types.Node, error)
	UncordonNode(ctx context.Context, id types.NodeID) (types.Node, error)
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
	Task        types.Task         `json:"task"`
	Healthcheck *types.Healthcheck `json:"healthcheck,omitempty"`
	Ports       []types.Port       `json:"ports,omitempty"`
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
	events      []types.Event
	now         func() time.Time
}

func NewMemoryService() *MemoryService {
	now := func() time.Time { return time.Now().UTC() }
	return &MemoryService{
		nodes:       make(map[types.NodeID]types.Node),
		services:    make(map[types.ServiceID]types.Service),
		versions:    make(map[types.ServiceID]map[int64]types.ServiceSpec),
		tasks:       make(map[types.TaskID]types.Task),
		deployments: make(map[types.DeploymentID]types.Deployment),
		now:         now,
	}
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
		node.Status = types.NodeOffline
	}
	node.LastHeartbeatAt = now
	node.UpdatedAt = now
	s.nodes[node.ID] = node

	eventType := events.TypeNodeHeartbeat
	message := "node heartbeat"
	if heartbeat.Shutdown {
		eventType = events.TypeNodeShutdown
		message = "node graceful shutdown"
	}
	s.appendEventLocked(eventType, types.EventInfo, "controlplane", message, "node", string(node.ID), now)
	return NodeCommand{Node: node, Directives: directivesForNode(node)}, nil
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
		if task.DesiredStatus == types.TaskRemoved || task.DesiredStatus == types.TaskStopped {
			continue
		}
		service := s.services[task.ServiceID]
		tasks = append(tasks, AgentTask{
			Task:        task,
			Healthcheck: service.Spec.Healthcheck,
			Ports:       service.Spec.Ports,
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
	if !validAgentTaskStatus(report.Status) {
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
	status := report.Status
	eventType := events.TypeTaskStatus
	severity := types.EventInfo
	message := "task status reported"
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
	return s.setNodeStatus(ctx, id, types.NodeDraining)
}

func (s *MemoryService) UncordonNode(ctx context.Context, id types.NodeID) (types.Node, error) {
	return s.setNodeStatus(ctx, id, types.NodeReady)
}

func (s *MemoryService) CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error) {
	if err := ctx.Err(); err != nil {
		return types.Service{}, err
	}
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

	now := s.now()
	service := types.Service{
		ID:                types.ServiceID(newUUID()),
		Spec:              spec,
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

	if _, ok := s.services[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.services, id)
	for taskID, task := range s.tasks {
		if task.ServiceID == id {
			delete(s.tasks, taskID)
		}
	}
	s.appendEventLocked(events.TypeServiceDeleted, types.EventInfo, "controlplane", "service deleted", "service", string(id), s.now())
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
	if active := s.activeRollbackLocked(id); active.ID != "" {
		return active, nil
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
	task.DesiredStatus = types.TaskStopped
	task.UpdatedAt = s.now()
	s.tasks[id] = task
	return task, nil
}

func (s *MemoryService) ListTasksByService(ctx context.Context, serviceID types.ServiceID) ([]types.Task, error) {
	return s.ListTasks(ctx, TaskFilter{ServiceID: serviceID})
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
	node.Status = status
	node.UpdatedAt = s.now()
	s.nodes[id] = node
	if status == types.NodeReady {
		s.reconcileAllServicesLocked(node.UpdatedAt)
	}
	s.appendEventLocked(events.TypeNodeStatusChanged, types.EventInfo, "controlplane", "node status changed", "node", string(id), node.UpdatedAt)
	return node, nil
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
	if s.hasActiveDeploymentLocked(service.ID) {
		return
	}
	active := make([]types.Task, 0)
	for _, task := range s.tasks {
		if task.ServiceID != service.ID {
			continue
		}
		if task.DesiredStatus == types.TaskRemoved || task.DesiredStatus == types.TaskStopped {
			continue
		}
		if task.Image != service.Spec.Image || task.Version != service.DeploymentVersion {
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
			CreatedAt:     timestamp,
			UpdatedAt:     timestamp,
		}
		s.tasks[task.ID] = task
		active = append(active, task)
	}

	for i, task := range active {
		if i >= service.Spec.Replicas {
			task.DesiredStatus = types.TaskRemoved
			task.UpdatedAt = timestamp
			s.tasks[task.ID] = task
			continue
		}
	}
}

func (s *MemoryService) hasActiveDeploymentLocked(serviceID types.ServiceID) bool {
	for _, deployment := range s.deployments {
		if deployment.ServiceID != serviceID {
			continue
		}
		if deployment.Status == types.DeploymentPending || deployment.Status == types.DeploymentRunning {
			return true
		}
	}
	return false
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

func validAgentTaskStatus(status types.TaskStatus) bool {
	switch status {
	case types.TaskPulling,
		types.TaskCreated,
		types.TaskRunning,
		types.TaskHealthy,
		types.TaskUnhealthy,
		types.TaskFailed,
		types.TaskStopped,
		types.TaskRemoved:
		return true
	default:
		return false
	}
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
