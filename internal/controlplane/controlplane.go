package controlplane

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

type Service interface {
	ListNodes(ctx context.Context) ([]types.Node, error)
	GetNode(ctx context.Context, id types.NodeID) (types.Node, error)
	DrainNode(ctx context.Context, id types.NodeID) (types.Node, error)
	UncordonNode(ctx context.Context, id types.NodeID) (types.Node, error)
	CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error)
	ListServices(ctx context.Context) ([]types.Service, error)
	GetService(ctx context.Context, id types.ServiceID) (types.Service, error)
	DeleteService(ctx context.Context, id types.ServiceID) error
	ScaleService(ctx context.Context, id types.ServiceID, replicas int) (types.Service, error)
	RolloutService(ctx context.Context, id types.ServiceID, image string) (types.Deployment, error)
	RollbackService(ctx context.Context, id types.ServiceID) (types.Deployment, error)
	ListTasks(ctx context.Context, filter TaskFilter) ([]types.Task, error)
	GetTask(ctx context.Context, id types.TaskID) (types.Task, error)
	ListEvents(ctx context.Context, filter EventFilter) ([]types.Event, error)
}

type TaskFilter struct {
	ServiceID types.ServiceID
	NodeID    types.NodeID
	Status    types.TaskStatus
}

type EventFilter struct {
	RelatedObjectType string
	RelatedObjectID   string
	Limit             int
}

type MemoryService struct {
	mu          sync.RWMutex
	nodes       map[types.NodeID]types.Node
	services    map[types.ServiceID]types.Service
	tasks       map[types.TaskID]types.Task
	deployments map[types.DeploymentID]types.Deployment
	events      []types.Event
	now         func() time.Time
}

func NewMemoryService() *MemoryService {
	now := func() time.Time { return time.Now().UTC() }
	return &MemoryService{
		nodes:       seedNodes(now),
		services:    make(map[types.ServiceID]types.Service),
		tasks:       make(map[types.TaskID]types.Task),
		deployments: make(map[types.DeploymentID]types.Deployment),
		now:         now,
	}
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
	s.appendEventLocked("service.created", types.EventInfo, "controlplane", "service created", "service", string(service.ID), now)
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
	s.appendEventLocked("service.deleted", types.EventInfo, "controlplane", "service deleted", "service", string(id), s.now())
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
	s.appendEventLocked("service.scaled", types.EventInfo, "controlplane", "service scaled", "service", string(id), service.UpdatedAt)
	return service, nil
}

func (s *MemoryService) RolloutService(ctx context.Context, id types.ServiceID, image string) (types.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return types.Deployment{}, err
	}
	if image == "" {
		return types.Deployment{}, fmt.Errorf("%w: image is required", store.ErrInvalidState)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	service, ok := s.services[id]
	if !ok {
		return types.Deployment{}, store.ErrNotFound
	}
	now := s.now()
	fromVersion := service.DeploymentVersion
	service.Spec.Image = image
	service.DeploymentVersion++
	service.UpdatedAt = now
	s.services[id] = service

	deployment := types.Deployment{
		ID:             types.DeploymentID(newUUID()),
		ServiceID:      id,
		FromVersion:    fromVersion,
		ToVersion:      service.DeploymentVersion,
		Strategy:       types.RolloutRollingUpdate,
		Status:         types.DeploymentPending,
		MaxUnavailable: 1,
		MaxSurge:       1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.deployments[deployment.ID] = deployment
	s.appendEventLocked("service.rollout.started", types.EventInfo, "controlplane", "service rollout started", "service", string(id), now)
	return deployment, nil
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
	if service.DeploymentVersion <= 1 {
		return types.Deployment{}, fmt.Errorf("%w: no previous deployment version", store.ErrInvalidState)
	}

	now := s.now()
	fromVersion := service.DeploymentVersion
	service.DeploymentVersion--
	service.UpdatedAt = now
	s.services[id] = service

	deployment := types.Deployment{
		ID:             types.DeploymentID(newUUID()),
		ServiceID:      id,
		FromVersion:    fromVersion,
		ToVersion:      service.DeploymentVersion,
		Strategy:       types.RolloutRollingUpdate,
		Status:         types.DeploymentRollingBack,
		MaxUnavailable: 1,
		MaxSurge:       0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.deployments[deployment.ID] = deployment
	s.appendEventLocked("service.rollback.started", types.EventInfo, "controlplane", "service rollback started", "service", string(id), now)
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

func (s *MemoryService) ListEvents(ctx context.Context, filter EventFilter) ([]types.Event, error) {
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
		if filter.RelatedObjectType != "" && event.RelatedObjectType != filter.RelatedObjectType {
			continue
		}
		if filter.RelatedObjectID != "" && event.RelatedObjectID != filter.RelatedObjectID {
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
	s.appendEventLocked("node.status.changed", types.EventInfo, "controlplane", "node status changed", "node", string(id), node.UpdatedAt)
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

func seedNodes(now func() time.Time) map[types.NodeID]types.Node {
	timestamp := now()
	id := types.NodeID("00000000-0000-4000-8000-000000000001")
	return map[types.NodeID]types.Node{
		id: {
			ID:               id,
			Hostname:         "local-node",
			AdvertiseAddress: "127.0.0.1",
			Labels:           map[string]string{"role": "worker"},
			Capacity:         types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
			Allocatable:      types.Resources{CPU: 3500, Memory: 7 * 1024 * 1024 * 1024},
			Status:           types.NodeReady,
			LastHeartbeatAt:  timestamp,
			CreatedAt:        timestamp,
			UpdatedAt:        timestamp,
		},
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
