package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

type Store interface {
	ListServices(ctx context.Context) ([]types.Service, error)
	ListTasksByService(ctx context.Context, serviceID types.ServiceID) ([]types.Task, error)
	ListTasksByStatus(ctx context.Context, status types.TaskStatus) ([]types.Task, error)
	ListDeploymentsByStatus(ctx context.Context, status types.DeploymentStatus) ([]types.Deployment, error)
	CreateTask(ctx context.Context, task types.Task) (types.Task, error)
	StopTask(ctx context.Context, id types.TaskID, expectedUpdatedAt time.Time) (types.Task, error)
	UpdateServiceStatus(ctx context.Context, id types.ServiceID, status types.ServiceStatus, expectedUpdatedAt time.Time) (types.Service, error)
	AppendEvent(ctx context.Context, event types.Event) (types.Event, error)
}

type LeaderLock interface {
	Acquire(ctx context.Context) (Lease, error)
}

type Lease interface {
	Release(ctx context.Context) error
}

type Metrics interface {
	IncReconciliationRuns()
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
	r.metrics.IncReconciliationRuns()
	if r.store == nil {
		r.metrics.IncReconciliationErrors()
		return fmt.Errorf("reconciler store is required")
	}
	if err := ctx.Err(); err != nil {
		r.metrics.IncReconciliationErrors()
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
	activeDeployments, err := r.activeDeploymentServices(ctx)
	if err != nil {
		return Result{}, err
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].ID < services[j].ID
	})

	knownServices := make(map[types.ServiceID]types.Service, len(services))
	result := Result{}
	for _, service := range services {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if service.Status == "" {
			service.Status = types.ServiceActive
		}
		if service.Status != types.ServiceDeleted {
			knownServices[service.ID] = service
		}
		var serviceResult Result
		var err error
		switch service.Status {
		case types.ServiceActive:
			if activeDeployments[service.ID] {
				continue
			}
			serviceResult, err = r.reconcileService(ctx, service)
		case types.ServiceDeleting:
			serviceResult, err = r.reconcileDeletingService(ctx, service)
		case types.ServiceDeleted:
			continue
		default:
			return result, fmt.Errorf("%w: service %s has invalid status %q", store.ErrInvalidState, service.ID, service.Status)
		}
		result.CreatedTasks += serviceResult.CreatedTasks
		result.StoppedTasks += serviceResult.StoppedTasks
		if err != nil {
			return result, err
		}
	}

	deletedResult, err := r.reconcileDeletedServices(ctx, knownServices)
	result.CreatedTasks += deletedResult.CreatedTasks
	result.StoppedTasks += deletedResult.StoppedTasks
	if err != nil {
		return result, err
	}
	return result, nil
}

func (r *Reconciler) activeDeploymentServices(ctx context.Context) (map[types.ServiceID]bool, error) {
	services := make(map[types.ServiceID]bool)
	for _, status := range []types.DeploymentStatus{
		types.DeploymentPending,
		types.DeploymentRunning,
		types.DeploymentRollingBack,
	} {
		deployments, err := r.store.ListDeploymentsByStatus(ctx, status)
		if err != nil {
			return nil, fmt.Errorf("list %s deployments: %w", status, err)
		}
		for _, deployment := range deployments {
			services[deployment.ServiceID] = true
		}
	}
	return services, nil
}

func (r *Reconciler) reconcileDeletingService(ctx context.Context, service types.Service) (Result, error) {
	tasks, err := r.store.ListTasksByService(ctx, service.ID)
	if err != nil {
		return Result{}, fmt.Errorf("list tasks for deleting service %s: %w", service.ID, err)
	}
	result := Result{}
	allRemoved := true
	for _, task := range sortedTasks(tasks) {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if task.ActualStatus == types.TaskRemoved {
			continue
		}
		allRemoved = false
		stopped, err := r.stopTask(ctx, task, "service is deleting")
		if err != nil {
			return result, err
		}
		if stopped {
			result.StoppedTasks++
		}
	}
	if allRemoved {
		transactional := r.transactional()
		var updated types.Service
		err := store.WithTx(ctx, r.store, func(txCtx context.Context, tx Store) error {
			var err error
			updated, err = tx.UpdateServiceStatus(txCtx, service.ID, types.ServiceDeleted, service.UpdatedAt)
			if err != nil {
				return err
			}
			return events.Emit(txCtx, tx, types.Event{
				Namespace:         updated.Namespace,
				Type:              events.TypeServiceDeleted,
				Severity:          types.EventInfo,
				Source:            "reconciler",
				Message:           "service deletion completed",
				RelatedObjectType: "service",
				RelatedObjectID:   string(updated.ID),
				Timestamp:         r.now(),
			}, eventOptions(transactional)...)
		})
		if err != nil {
			if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
				return result, nil
			}
			return result, fmt.Errorf("mark service %s deleted: %w", service.ID, err)
		}
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
		stopped, err := r.stopTask(ctx, task, "task version is no longer current")
		if err != nil {
			return result, err
		}
		if stopped {
			result.StoppedTasks++
		}
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
			stopped, err := r.stopTask(ctx, task, "service replicas above desired count")
			if err != nil {
				return result, err
			}
			if stopped {
				result.StoppedTasks++
			}
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
			stopped, err := r.stopTask(ctx, task, "service no longer exists")
			if err != nil {
				if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
					continue
				}
				return result, err
			}
			if stopped {
				result.StoppedTasks++
			}
		}
	}
	return result, nil
}

func (r *Reconciler) createTask(ctx context.Context, service types.Service, reason string) error {
	task := types.Task{
		Namespace:           service.Namespace,
		ServiceID:           service.ID,
		DesiredStatus:       types.TaskRunning,
		ActualStatus:        types.TaskPending,
		Image:               service.Spec.EffectiveImage(),
		RequestedImage:      service.Spec.ImageMetadata.RequestedImage,
		ResolvedImageDigest: service.Spec.ImageMetadata.Digest,
		ImageRegistry:       service.Spec.ImageMetadata.Registry,
		ImageName:           service.Spec.ImageMetadata.Name,
		ImageTag:            service.Spec.ImageMetadata.Tag,
		Version:             service.DeploymentVersion,
	}
	transactional := r.transactional()
	err := store.WithTx(ctx, r.store, func(txCtx context.Context, tx Store) error {
		created, err := tx.CreateTask(txCtx, task)
		if err != nil {
			return err
		}
		return events.Emit(txCtx, tx, types.Event{
			Namespace:         service.Namespace,
			Type:              events.TypeReconcilerTaskCreated,
			Severity:          types.EventInfo,
			Source:            "reconciler",
			Message:           reason,
			RelatedObjectType: "task",
			RelatedObjectID:   string(created.ID),
			Timestamp:         r.now(),
		}, eventOptions(transactional)...)
	})
	if err != nil {
		return fmt.Errorf("create task for service %s: %w", service.ID, err)
	}
	return nil
}

func (r *Reconciler) stopTask(ctx context.Context, task types.Task, reason string) (bool, error) {
	if task.DesiredStatus == types.TaskStopped || task.DesiredStatus == types.TaskRemoved {
		return false, nil
	}
	transactional := r.transactional()
	stopped := false
	err := store.WithTx(ctx, r.store, func(txCtx context.Context, tx Store) error {
		stoppedTask, err := tx.StopTask(txCtx, task.ID, task.UpdatedAt)
		if err != nil {
			return err
		}
		if err := events.Emit(txCtx, tx, types.Event{
			Namespace:         task.Namespace,
			Type:              events.TypeReconcilerTaskStopped,
			Severity:          types.EventInfo,
			Source:            "reconciler",
			Message:           reason,
			RelatedObjectType: "task",
			RelatedObjectID:   string(stoppedTask.ID),
			Timestamp:         r.now(),
		}, eventOptions(transactional)...); err != nil {
			return err
		}
		stopped = true
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("stop task %s: %w", task.ID, err)
	}
	return stopped, nil
}

func (r *Reconciler) transactional() bool {
	_, ok := any(r.store).(store.Transactor)
	return ok
}

func eventOptions(transactional bool) []events.EmitOption {
	if transactional {
		return []events.EmitOption{events.Strict()}
	}
	return nil
}

func isNonTerminal(task types.Task) bool {
	return types.IsActiveTask(task)
}

func nonTerminalStatuses() []types.TaskStatus {
	return types.NonTerminalTaskStatuses()
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

func (NoopMetrics) IncReconciliationRuns() {}

func (NoopMetrics) ObserveReconciliationDuration(time.Duration) {}
func (NoopMetrics) IncReconciliationErrors()                    {}
func (NoopMetrics) AddCreatedTasks(int)                         {}
func (NoopMetrics) AddStoppedTasks(int)                         {}
