package rollout

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

type Store interface {
	GetService(ctx context.Context, id types.ServiceID) (types.Service, error)
	ListTasksByService(ctx context.Context, serviceID types.ServiceID) ([]types.Task, error)
	CreateTask(ctx context.Context, task types.Task) (types.Task, error)
	StopTask(ctx context.Context, id types.TaskID, expectedUpdatedAt time.Time) (types.Task, error)
	ListDeploymentsByStatus(ctx context.Context, status types.DeploymentStatus) ([]types.Deployment, error)
	UpdateDeploymentStatus(ctx context.Context, id types.DeploymentID, status types.DeploymentStatus, expectedUpdatedAt time.Time) (types.Deployment, error)
	AppendEvent(ctx context.Context, event types.Event) (types.Event, error)
}

type Controller struct {
	store   Store
	metrics Metrics
	logger  *slog.Logger
	now     func() time.Time
}

type Metrics interface {
	AddCreatedTasks(count int)
	IncRolloutFailures()
}

type NoopMetrics struct{}

func (NoopMetrics) AddCreatedTasks(int) {}

func (NoopMetrics) IncRolloutFailures() {}

type Option func(*Controller)

func WithMetrics(metrics Metrics) Option {
	return func(c *Controller) {
		if metrics != nil {
			c.metrics = metrics
		}
	}
}

func NewController(store Store, logger *slog.Logger, opts ...Option) *Controller {
	if logger == nil {
		logger = slog.Default()
	}
	c := &Controller{
		store:   store,
		metrics: NoopMetrics{},
		logger:  logger,
		now:     func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Controller) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := c.RunOnce(ctx); err != nil {
			c.logger.Warn("rollout controller pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Controller) RunOnce(ctx context.Context) error {
	if c.store == nil {
		return fmt.Errorf("rollout store is required")
	}
	deployments, err := c.activeDeployments(ctx)
	if err != nil {
		return err
	}
	for _, deployment := range deployments {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.reconcileDeployment(ctx, deployment); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) activeDeployments(ctx context.Context) ([]types.Deployment, error) {
	var deployments []types.Deployment
	for _, status := range []types.DeploymentStatus{types.DeploymentPending, types.DeploymentRunning, types.DeploymentRollingBack} {
		items, err := c.store.ListDeploymentsByStatus(ctx, status)
		if err != nil {
			return nil, fmt.Errorf("list %s deployments: %w", status, err)
		}
		deployments = append(deployments, items...)
	}
	sort.Slice(deployments, func(i, j int) bool {
		if deployments[i].CreatedAt.Equal(deployments[j].CreatedAt) {
			return deployments[i].ID < deployments[j].ID
		}
		return deployments[i].CreatedAt.Before(deployments[j].CreatedAt)
	})
	return deployments, nil
}

func (c *Controller) reconcileDeployment(ctx context.Context, deployment types.Deployment) error {
	service, err := c.store.GetService(ctx, deployment.ServiceID)
	if err != nil {
		return fmt.Errorf("get rollout service %s: %w", deployment.ServiceID, err)
	}
	tasks, err := c.store.ListTasksByService(ctx, deployment.ServiceID)
	if err != nil {
		return fmt.Errorf("list rollout tasks for service %s: %w", deployment.ServiceID, err)
	}

	state := rolloutStateFor(deployment, service, tasks)
	if state.newFailed > 0 {
		_, err := c.setStatus(ctx, deployment, types.DeploymentFailed, "rollout failed because a new task failed health checks", types.EventError)
		return err
	}
	if state.oldActive == 0 && state.newHealthy >= service.Spec.Replicas {
		_, err := c.setStatus(ctx, deployment, terminalStatusFor(deployment), terminalMessageFor(deployment), types.EventInfo)
		return err
	}
	if deployment.Status == types.DeploymentPending {
		updated, err := c.setStatus(ctx, deployment, types.DeploymentRunning, "rollout started", types.EventInfo)
		if err != nil {
			return err
		}
		deployment = updated
	}

	created, err := c.createNewTasks(ctx, deployment, service, state)
	if err != nil {
		return err
	}
	c.metrics.AddCreatedTasks(created)
	state.newActive += created
	state.totalActive += created

	stopped, err := c.stopOldTasks(ctx, deployment, service, state)
	if err != nil {
		return err
	}
	state.oldActive -= stopped
	state.totalActive -= stopped
	if stopped > 0 || created > 0 {
		_ = events.Emit(ctx, c.store, types.Event{
			Type:              events.TypeRolloutAdvanced,
			Severity:          types.EventInfo,
			Source:            "rollout",
			Message:           fmt.Sprintf("rollout advanced: created=%d stopped=%d", created, stopped),
			RelatedObjectType: "service",
			RelatedObjectID:   string(deployment.ServiceID),
			Timestamp:         c.now(),
		}, events.WithLogger(c.logger))
	}
	if state.oldActive == 0 && state.newHealthy >= service.Spec.Replicas {
		_, err := c.setStatus(ctx, deployment, terminalStatusFor(deployment), terminalMessageFor(deployment), types.EventInfo)
		return err
	}
	return nil
}

func (c *Controller) createNewTasks(ctx context.Context, deployment types.Deployment, service types.Service, state rolloutState) (int, error) {
	created := 0
	surgeLimit := service.Spec.Replicas + deployment.MaxSurge
	if surgeLimit < service.Spec.Replicas {
		surgeLimit = service.Spec.Replicas
	}
	for state.newActive+created < service.Spec.Replicas && state.totalActive+created < surgeLimit {
		task, err := c.store.CreateTask(ctx, types.Task{
			ServiceID:     service.ID,
			DesiredStatus: types.TaskRunning,
			ActualStatus:  types.TaskPending,
			Image:         service.Spec.Image,
			Version:       deployment.ToVersion,
		})
		if err != nil {
			return created, fmt.Errorf("create rollout task: %w", err)
		}
		created++
		_ = events.Emit(ctx, c.store, types.Event{
			Type:              events.TypeRolloutTaskCreated,
			Severity:          types.EventInfo,
			Source:            "rollout",
			Message:           "created new-version task",
			RelatedObjectType: "task",
			RelatedObjectID:   string(task.ID),
			Timestamp:         c.now(),
		}, events.WithLogger(c.logger))
	}
	return created, nil
}

func (c *Controller) stopOldTasks(ctx context.Context, deployment types.Deployment, service types.Service, state rolloutState) (int, error) {
	stopped := 0
	oldTasks := append([]types.Task(nil), state.oldTasks...)
	sort.Slice(oldTasks, func(i, j int) bool {
		return oldTasks[i].ID > oldTasks[j].ID
	})
	for _, task := range oldTasks {
		if state.newHealthy <= state.oldStopped+stopped {
			break
		}
		if service.Spec.Replicas-(state.available-stopped-1) > deployment.MaxUnavailable {
			break
		}
		stoppedTask, err := c.store.StopTask(ctx, task.ID, task.UpdatedAt)
		if err != nil {
			return stopped, fmt.Errorf("stop old rollout task %s: %w", task.ID, err)
		}
		stopped++
		_ = events.Emit(ctx, c.store, types.Event{
			Type:              events.TypeRolloutTaskStopped,
			Severity:          types.EventInfo,
			Source:            "rollout",
			Message:           "stopped old-version task",
			RelatedObjectType: "task",
			RelatedObjectID:   string(stoppedTask.ID),
			Timestamp:         c.now(),
		}, events.WithLogger(c.logger))
	}
	return stopped, nil
}

func (c *Controller) setStatus(ctx context.Context, deployment types.Deployment, status types.DeploymentStatus, message string, severity types.EventSeverity) (types.Deployment, error) {
	if deployment.Status == status {
		return deployment, nil
	}
	updated, err := c.store.UpdateDeploymentStatus(ctx, deployment.ID, status, deployment.UpdatedAt)
	if err != nil {
		return types.Deployment{}, fmt.Errorf("update rollout status: %w", err)
	}
	deployment = updated
	if status == types.DeploymentFailed {
		c.metrics.IncRolloutFailures()
	}
	_ = events.Emit(ctx, c.store, types.Event{
		Type:              events.TypeRolloutStatusChanged,
		Severity:          severity,
		Source:            "rollout",
		Message:           message,
		RelatedObjectType: "service",
		RelatedObjectID:   string(deployment.ServiceID),
		Timestamp:         c.now(),
	}, events.WithLogger(c.logger))
	return deployment, nil
}

type rolloutState struct {
	oldTasks    []types.Task
	oldActive   int
	oldStopped  int
	newActive   int
	newHealthy  int
	newFailed   int
	totalActive int
	available   int
}

func rolloutStateFor(deployment types.Deployment, service types.Service, tasks []types.Task) rolloutState {
	state := rolloutState{}
	for _, task := range tasks {
		if task.Version == deployment.FromVersion {
			if isActive(task) {
				state.oldActive++
				state.oldTasks = append(state.oldTasks, task)
				if isAvailable(task) {
					state.available++
				}
			} else {
				state.oldStopped++
			}
			continue
		}
		if task.Version != deployment.ToVersion {
			continue
		}
		if task.ActualStatus == types.TaskFailed {
			state.newFailed++
		}
		if isActive(task) {
			state.newActive++
			if task.ActualStatus == types.TaskHealthy {
				state.newHealthy++
				state.available++
			}
		}
	}
	state.totalActive = state.oldActive + state.newActive
	if state.oldActive == 0 && state.oldStopped == 0 {
		state.oldStopped = service.Spec.Replicas
	}
	return state
}

func isActive(task types.Task) bool {
	return types.IsActiveTask(task)
}

func isAvailable(task types.Task) bool {
	return types.IsAvailableTaskStatus(task.ActualStatus)
}

func terminalStatusFor(deployment types.Deployment) types.DeploymentStatus {
	if deployment.ToVersion < deployment.FromVersion {
		return types.DeploymentRolledBack
	}
	return types.DeploymentSucceeded
}

func terminalMessageFor(deployment types.Deployment) string {
	if deployment.ToVersion < deployment.FromVersion {
		return "rollback completed"
	}
	return "rollout completed"
}
