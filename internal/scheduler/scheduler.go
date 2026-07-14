package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

type Store interface {
	ListTasksByStatus(ctx context.Context, status types.TaskStatus) ([]types.Task, error)
	ListNodesByStatus(ctx context.Context, status types.NodeStatus) ([]types.Node, error)
	ListTasksByNode(ctx context.Context, nodeID types.NodeID) ([]types.Task, error)
	GetService(ctx context.Context, id types.ServiceID) (types.Service, error)
	AssignTask(ctx context.Context, id types.TaskID, nodeID types.NodeID, expectedUpdatedAt time.Time) (types.Task, error)
	AppendEvent(ctx context.Context, event types.Event) (types.Event, error)
}

type Assignment struct {
	TaskID types.TaskID `json:"task_id"`
	NodeID types.NodeID `json:"node_id"`
}

type PlanInput struct {
	PendingTasks []types.Task
	Nodes        []types.Node
	RunningTasks []types.Task
	Services     map[types.ServiceID]types.Service
}

type Scheduler struct {
	store   Store
	metrics Metrics
	now     func() time.Time
}

type Metrics interface {
	IncSchedulerRuns()
	IncSchedulerErrors()
	ObserveSchedulerDuration(duration time.Duration)
}

type NoopMetrics struct{}

func (NoopMetrics) IncSchedulerRuns() {}

func (NoopMetrics) IncSchedulerErrors() {}

func (NoopMetrics) ObserveSchedulerDuration(time.Duration) {}

type Option func(*Scheduler)

func WithMetrics(metrics Metrics) Option {
	return func(s *Scheduler) {
		if metrics != nil {
			s.metrics = metrics
		}
	}
}

func New(store Store, opts ...Option) *Scheduler {
	s := &Scheduler{
		store:   store,
		metrics: NoopMetrics{},
		now:     func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Scheduler) RunOnce(ctx context.Context) ([]Assignment, error) {
	started := s.now()
	s.metrics.IncSchedulerRuns()
	defer func() {
		s.metrics.ObserveSchedulerDuration(s.now().Sub(started))
	}()
	if s.store == nil {
		s.metrics.IncSchedulerErrors()
		return nil, fmt.Errorf("scheduler store is required")
	}
	if err := ctx.Err(); err != nil {
		s.metrics.IncSchedulerErrors()
		return nil, err
	}

	pending, err := s.store.ListTasksByStatus(ctx, types.TaskPending)
	if err != nil {
		s.metrics.IncSchedulerErrors()
		return nil, fmt.Errorf("list pending tasks: %w", err)
	}
	nodes, err := s.store.ListNodesByStatus(ctx, types.NodeReady)
	if err != nil {
		s.metrics.IncSchedulerErrors()
		return nil, fmt.Errorf("list ready nodes: %w", err)
	}
	running, err := s.loadRunningTasks(ctx, nodes)
	if err != nil {
		s.metrics.IncSchedulerErrors()
		return nil, err
	}
	services, err := s.loadServices(ctx, pending, running)
	if err != nil {
		s.metrics.IncSchedulerErrors()
		return nil, err
	}

	planned := Plan(PlanInput{
		PendingTasks: pending,
		Nodes:        nodes,
		RunningTasks: running,
		Services:     services,
	})
	assignments := make([]Assignment, 0, len(planned))
	for _, assignment := range planned {
		task, ok := taskByID(pending, assignment.TaskID)
		if !ok {
			continue
		}
		if _, err := s.store.AssignTask(ctx, assignment.TaskID, assignment.NodeID, task.UpdatedAt); err != nil {
			if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
				continue
			}
			s.metrics.IncSchedulerErrors()
			return assignments, fmt.Errorf("assign task %s: %w", assignment.TaskID, err)
		}
		assignments = append(assignments, assignment)
		_ = events.Emit(ctx, s.store, types.Event{
			Type:              events.TypeTaskAssigned,
			Severity:          types.EventInfo,
			Source:            "scheduler",
			Message:           "task assigned to node",
			RelatedObjectType: "task",
			RelatedObjectID:   string(assignment.TaskID),
			Timestamp:         s.now(),
		})
	}
	return assignments, nil
}

func (s *Scheduler) loadRunningTasks(ctx context.Context, nodes []types.Node) ([]types.Task, error) {
	running := make([]types.Task, 0)
	for _, node := range sortedNodes(nodes) {
		tasks, err := s.store.ListTasksByNode(ctx, node.ID)
		if err != nil {
			return nil, fmt.Errorf("list tasks for node %s: %w", node.ID, err)
		}
		for _, task := range tasks {
			if consumesResources(task) {
				running = append(running, task)
			}
		}
	}
	return running, nil
}

func consumesResources(task types.Task) bool {
	if task.NodeID == "" {
		return false
	}
	return types.IsActiveTask(task)
}

func (s *Scheduler) loadServices(ctx context.Context, taskSets ...[]types.Task) (map[types.ServiceID]types.Service, error) {
	services := make(map[types.ServiceID]types.Service)
	for _, tasks := range taskSets {
		for _, task := range tasks {
			if task.ServiceID == "" {
				continue
			}
			if _, ok := services[task.ServiceID]; ok {
				continue
			}
			service, err := s.store.GetService(ctx, task.ServiceID)
			if err != nil {
				return nil, fmt.Errorf("get service %s: %w", task.ServiceID, err)
			}
			services[task.ServiceID] = service
		}
	}
	return services, nil
}

func Plan(input PlanInput) []Assignment {
	pending := sortedTasks(input.PendingTasks)
	nodes := sortedNodes(input.Nodes)
	state := newClusterState(nodes, input.RunningTasks, input.Services)

	assignments := make([]Assignment, 0, len(pending))
	for _, task := range pending {
		if task.ActualStatus != types.TaskPending {
			continue
		}
		service, ok := input.Services[task.ServiceID]
		if !ok {
			continue
		}
		node, ok := bestNode(task, service, nodes, state)
		if !ok {
			continue
		}
		assignments = append(assignments, Assignment{TaskID: task.ID, NodeID: node.ID})
		state.addPlanned(node.ID, task.ServiceID, service.Spec.ResourceRequirements.Requests)
	}
	return assignments
}

type clusterState struct {
	usedByNode           map[types.NodeID]types.Resources
	runningByNode        map[types.NodeID]int
	runningByNodeService map[types.NodeID]map[types.ServiceID]int
}

func newClusterState(nodes []types.Node, runningTasks []types.Task, services map[types.ServiceID]types.Service) *clusterState {
	state := &clusterState{
		usedByNode:           make(map[types.NodeID]types.Resources, len(nodes)),
		runningByNode:        make(map[types.NodeID]int, len(nodes)),
		runningByNodeService: make(map[types.NodeID]map[types.ServiceID]int, len(nodes)),
	}
	knownNodes := make(map[types.NodeID]struct{}, len(nodes))
	for _, node := range nodes {
		knownNodes[node.ID] = struct{}{}
		state.runningByNodeService[node.ID] = make(map[types.ServiceID]int)
	}
	for _, task := range sortedTasks(runningTasks) {
		if _, ok := knownNodes[task.NodeID]; !ok {
			continue
		}
		service, ok := services[task.ServiceID]
		if !ok {
			continue
		}
		state.addPlanned(task.NodeID, task.ServiceID, service.Spec.ResourceRequirements.Requests)
	}
	return state
}

func (s *clusterState) addPlanned(nodeID types.NodeID, serviceID types.ServiceID, requests types.Resources) {
	used := s.usedByNode[nodeID]
	used.CPU += requests.CPU
	used.Memory += requests.Memory
	s.usedByNode[nodeID] = used
	s.runningByNode[nodeID]++
	if s.runningByNodeService[nodeID] == nil {
		s.runningByNodeService[nodeID] = make(map[types.ServiceID]int)
	}
	s.runningByNodeService[nodeID][serviceID]++
}

func bestNode(task types.Task, service types.Service, nodes []types.Node, state *clusterState) (types.Node, bool) {
	var best types.Node
	var bestScore nodeScore
	found := false
	requests := service.Spec.ResourceRequirements.Requests
	for _, node := range nodes {
		if !fits(task, service, node, state, requests) {
			continue
		}
		score := scoreNode(node, task.ServiceID, state, requests)
		if !found || score.less(bestScore) {
			best = node
			bestScore = score
			found = true
		}
	}
	return best, found
}

func fits(_ types.Task, service types.Service, node types.Node, state *clusterState, requests types.Resources) bool {
	if node.Status != types.NodeReady {
		return false
	}
	if !placementMatches(service.Spec.PlacementConstraints, node.Labels) {
		return false
	}
	free := freeResources(node, state)
	return free.CPU >= requests.CPU && free.Memory >= requests.Memory
}

type nodeScore struct {
	freeMemoryAfter  int64
	serviceTaskCount int
	totalTaskCount   int
	nodeID           types.NodeID
}

func scoreNode(node types.Node, serviceID types.ServiceID, state *clusterState, requests types.Resources) nodeScore {
	free := freeResources(node, state)
	return nodeScore{
		freeMemoryAfter:  free.Memory - requests.Memory,
		serviceTaskCount: state.runningByNodeService[node.ID][serviceID],
		totalTaskCount:   state.runningByNode[node.ID],
		nodeID:           node.ID,
	}
}

func (s nodeScore) less(other nodeScore) bool {
	if s.freeMemoryAfter != other.freeMemoryAfter {
		return s.freeMemoryAfter > other.freeMemoryAfter
	}
	if s.serviceTaskCount != other.serviceTaskCount {
		return s.serviceTaskCount < other.serviceTaskCount
	}
	if s.totalTaskCount != other.totalTaskCount {
		return s.totalTaskCount < other.totalTaskCount
	}
	return s.nodeID < other.nodeID
}

func freeResources(node types.Node, state *clusterState) types.Resources {
	used := state.usedByNode[node.ID]
	return types.Resources{
		CPU:    node.Allocatable.CPU - used.CPU,
		Memory: node.Allocatable.Memory - used.Memory,
	}
}

func placementMatches(constraints []types.PlacementConstraint, labels map[string]string) bool {
	for _, constraint := range constraints {
		value, ok := labels[constraint.Key]
		switch constraint.Operator {
		case types.ConstraintEquals:
			if !ok || value != constraint.Value {
				return false
			}
		case types.ConstraintNotEquals:
			if ok && value == constraint.Value {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func sortedTasks(tasks []types.Task) []types.Task {
	ordered := append([]types.Task(nil), tasks...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

func sortedNodes(nodes []types.Node) []types.Node {
	ordered := append([]types.Node(nil), nodes...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

func taskByID(tasks []types.Task, id types.TaskID) (types.Task, bool) {
	for _, task := range tasks {
		if task.ID == id {
			return task, true
		}
	}
	return types.Task{}, false
}
