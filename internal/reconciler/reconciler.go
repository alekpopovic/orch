package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

type Store interface {
	ListServices(ctx context.Context) ([]types.Service, error)
	ListTasksByService(ctx context.Context, serviceID types.ServiceID) ([]types.Task, error)
	ListTasksByStatus(ctx context.Context, status types.TaskStatus) ([]types.Task, error)
	CreateTask(ctx context.Context, task types.Task) (types.Task, error)
	StopTask(ctx context.Context, id types.TaskID, expectedUpdatedAt time.Time) (types.Task, error)
	AppendEvent(ctx context.Context, event types.Event) (types.Event, error)
}

type LeaderLock interface {
	Acquire(ctx context.Context) (Lease, error)
}

type Lease interface {
	Release(ctx context.Context) error
}

type Metrics interface {
	ObserveReconciliationDuration(duration time.Duration)
	IncReconciliationErrors()
	AddCreatedTasks(count int)
	AddStoppedTasks(count int)
}

type Config struct {
	Interval time.Duration
}

type Option func(*Reconciler)

type Reconciler struct {
	store   Store
	lock    LeaderLock
	metrics Metrics
	logger  *slog.Logger
	now     func() time.Time
	config  Config
}

func New(store Store, opts ...Option) *Reconciler {
	r := &Reconciler{
		store:   store,
		lock:    NoopLeaderLock{},
		metrics: NoopMetrics{},
		logger:  slog.Default(),
		now:     func() time.Time { return time.Now().UTC() },
		config:  Config{Interval: 15 * time.Second},
	}
	for _, opt := range opts {
		opt(r)
	}
	if r.config.Interval <= 0 {
		r.config.Interval = 15 * time.Second
	}
	return r
}

func WithInterval(interval time.Duration) Option {
	return func(r *Reconciler) {
		r.config.Interval = interval
	}
}

func WithLeaderLock(lock LeaderLock) Option {
	return func(r *Reconciler) {
		if lock != nil {
			r.lock = lock
		}
	}
}

func WithMetrics(metrics Metrics) Option {
	return func(r *Reconciler) {
		if metrics != nil {
			r.metrics = metrics
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(r *Reconciler) {
		if logger != nil {
			r.logger = logger
		}
	}
}

func (r *Reconciler) Run(ctx context.Context) error {
	if err := r.ReconcileOnce(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(r.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.ReconcileOnce(ctx); err != nil {
				r.logger.Warn("service reconciliation failed", "error", err)
			}
		}
	}
}

func (r *Reconciler) ReconcileOnce(ctx context.Context) error {
	if r.store == nil {
		return fmt.Errorf("reconciler store is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	started := r.now()
	lease, err := r.lock.Acquire(ctx)
	if err != nil {
		r.metrics.IncReconciliationErrors()
		return fmt.Errorf("acquire leader lock: %w", err)
	}
	defer func() {
		if lease != nil {
			if err := lease.Release(context.Background()); err != nil {
				r.logger.Warn("release reconciler leader lock failed", "error", err)
			}
		}
	}()

	result, err := r.reconcile(ctx)
	r.metrics.ObserveReconciliationDuration(r.now().Sub(started))
	if err != nil {
		r.metrics.IncReconciliationErrors()
		return err
	}
	r.metrics.AddCreatedTasks(result.CreatedTasks)
	r.metrics.AddStoppedTasks(result.StoppedTasks)
	return nil
}

type Result struct {
	CreatedTasks int
	StoppedTasks int
}

func (r *Reconciler) reconcile(ctx context.Context) (Result, error) {
	services, err := r.store.ListServices(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list services: %w", err)
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].ID < services[j].ID
	})

	activeServices := make(map[types.ServiceID]types.Service, len(services))
	result := Result{}
	for _, service := range services {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		activeServices[service.ID] = service
		serviceResult, err := r.reconcileService(ctx, service)
		result.CreatedTasks += serviceResult.CreatedTasks
		result.StoppedTasks += serviceResult.StoppedTasks
		if err != nil {
			return result, err
		}
	}

	deletedResult, err := r.reconcileDeletedServices(ctx, activeServices)
	result.CreatedTasks += deletedResult.CreatedTasks
	result.StoppedTasks += deletedResult.StoppedTasks
	if err != nil {
		return result, err
	}
	return result, nil
}

func (r *Reconciler) reconcileService(ctx context.Context, service types.Service) (Result, error) {
	tasks, err := r.store.ListTasksByService(ctx, service.ID)
	if err != nil {
		return Result{}, fmt.Errorf("list tasks for service %s: %w", service.ID, err)
	}
	tasks = sortedTasks(tasks)

	result := Result{}
	current := make([]types.Task, 0)
	outdated := make([]types.Task, 0)
	nonRestartableFailed := 0
	for _, task := range tasks {
		if task.Version != service.DeploymentVersion {
			if isNonTerminal(task) {
				outdated = append(outdated, task)
			}
			continue
		}
		if isNonTerminal(task) {
			current = append(current, task)
			continue
		}
		if task.ActualStatus == types.TaskFailed && !restartAllowed(service.Spec.RestartPolicy) {
			nonRestartableFailed++
		}
	}

	for _, task := range stopOrder(outdated) {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := r.stopTask(ctx, task, "task version is no longer current"); err != nil {
			return result, err
		}
		result.StoppedTasks++
	}

	effectiveCount := len(current) + nonRestartableFailed
	if effectiveCount < service.Spec.Replicas {
		missing := service.Spec.Replicas - effectiveCount
		for i := 0; i < missing; i++ {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if err := r.createTask(ctx, service, "service replicas below desired count"); err != nil {
				return result, err
			}
			result.CreatedTasks++
		}
		return result, nil
	}

	if len(current) > service.Spec.Replicas {
		extra := len(current) - service.Spec.Replicas
		for _, task := range stopOrder(current)[:extra] {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if err := r.stopTask(ctx, task, "service replicas above desired count"); err != nil {
				return result, err
			}
			result.StoppedTasks++
		}
	}
	return result, nil
}

func (r *Reconciler) reconcileDeletedServices(ctx context.Context, activeServices map[types.ServiceID]types.Service) (Result, error) {
	result := Result{}
	for _, status := range nonTerminalStatuses() {
		tasks, err := r.store.ListTasksByStatus(ctx, status)
		if err != nil {
			return result, fmt.Errorf("list %s tasks: %w", status, err)
		}
		for _, task := range sortedTasks(tasks) {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if !isNonTerminal(task) {
				continue
			}
			if _, ok := activeServices[task.ServiceID]; ok {
				continue
			}
			if err := r.stopTask(ctx, task, "service no longer exists"); err != nil {
				if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
					continue
				}
				return result, err
			}
			result.StoppedTasks++
		}
	}
	return result, nil
}

func (r *Reconciler) createTask(ctx context.Context, service types.Service, reason string) error {
	task := types.Task{
		ServiceID:     service.ID,
		DesiredStatus: types.TaskRunning,
		ActualStatus:  types.TaskPending,
		Image:         service.Spec.Image,
		Version:       service.DeploymentVersion,
	}
	created, err := r.store.CreateTask(ctx, task)
	if err != nil {
		return fmt.Errorf("create task for service %s: %w", service.ID, err)
	}
	if _, err := r.store.AppendEvent(ctx, types.Event{
		Type:              "reconciler.task.created",
		Severity:          types.EventInfo,
		Source:            "reconciler",
		Message:           reason,
		RelatedObjectType: "task",
		RelatedObjectID:   string(created.ID),
		Timestamp:         r.now(),
	}); err != nil {
		return fmt.Errorf("append task created event: %w", err)
	}
	return nil
}

func (r *Reconciler) stopTask(ctx context.Context, task types.Task, reason string) error {
	if task.DesiredStatus == types.TaskStopped || task.DesiredStatus == types.TaskRemoved {
		return nil
	}
	stopped, err := r.store.StopTask(ctx, task.ID, task.UpdatedAt)
	if err != nil {
		return fmt.Errorf("stop task %s: %w", task.ID, err)
	}
	if _, err := r.store.AppendEvent(ctx, types.Event{
		Type:              "reconciler.task.stopped",
		Severity:          types.EventInfo,
		Source:            "reconciler",
		Message:           reason,
		RelatedObjectType: "task",
		RelatedObjectID:   string(stopped.ID),
		Timestamp:         r.now(),
	}); err != nil {
		return fmt.Errorf("append task stopped event: %w", err)
	}
	return nil
}

func isNonTerminal(task types.Task) bool {
	if task.DesiredStatus == types.TaskStopped || task.DesiredStatus == types.TaskRemoved {
		return false
	}
	switch task.ActualStatus {
	case types.TaskPending,
		types.TaskAssigned,
		types.TaskPulling,
		types.TaskCreated,
		types.TaskStarting,
		types.TaskRunning,
		types.TaskHealthy,
		types.TaskUnhealthy,
		types.TaskStopping:
		return true
	default:
		return false
	}
}

func nonTerminalStatuses() []types.TaskStatus {
	return []types.TaskStatus{
		types.TaskPending,
		types.TaskAssigned,
		types.TaskPulling,
		types.TaskCreated,
		types.TaskStarting,
		types.TaskRunning,
		types.TaskHealthy,
		types.TaskUnhealthy,
		types.TaskStopping,
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

func stopOrder(tasks []types.Task) []types.Task {
	ordered := sortedTasks(tasks)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftPriority := stopPriority(ordered[i])
		rightPriority := stopPriority(ordered[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return ordered[i].ID > ordered[j].ID
	})
	return ordered
}

func stopPriority(task types.Task) int {
	switch task.ActualStatus {
	case types.TaskPending:
		return 0
	case types.TaskAssigned:
		return 1
	case types.TaskPulling, types.TaskCreated, types.TaskStarting:
		return 2
	case types.TaskUnhealthy:
		return 3
	case types.TaskRunning, types.TaskHealthy:
		return 4
	case types.TaskStopping:
		return 5
	default:
		return 6
	}
}

func sortedTasks(tasks []types.Task) []types.Task {
	ordered := append([]types.Task(nil), tasks...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

type NoopLeaderLock struct{}

func (NoopLeaderLock) Acquire(context.Context) (Lease, error) {
	return noopLease{}, nil
}

type noopLease struct{}

func (noopLease) Release(context.Context) error {
	return nil
}

type NoopMetrics struct{}

func (NoopMetrics) ObserveReconciliationDuration(time.Duration) {}
func (NoopMetrics) IncReconciliationErrors()                    {}
func (NoopMetrics) AddCreatedTasks(int)                         {}
func (NoopMetrics) AddStoppedTasks(int)                         {}
