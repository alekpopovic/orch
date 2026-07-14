package autoscaler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

const defaultCooldown = time.Minute

type ControlPlane interface {
	ListServices(ctx context.Context) ([]types.Service, error)
	ScaleService(ctx context.Context, id types.ServiceID, replicas int) (types.Service, error)
	GetServiceRollout(ctx context.Context, id types.ServiceID) (types.Deployment, error)
}

type MetricsProvider interface {
	ServiceCPUUtilization(ctx context.Context, service types.Service) (float64, error)
}

type EventStore interface {
	AppendEvent(ctx context.Context, event types.Event) (types.Event, error)
}

type Metrics interface {
	IncAutoscalerDecision(decision string)
	IncAutoscalerErrors()
	SetAutoscalerRecommendation(serviceID string, replicas int)
}

type NoopMetrics struct{}

func (NoopMetrics) IncAutoscalerDecision(string)            {}
func (NoopMetrics) IncAutoscalerErrors()                    {}
func (NoopMetrics) SetAutoscalerRecommendation(string, int) {}

type Controller struct {
	controlPlane ControlPlane
	provider     MetricsProvider
	eventStore   EventStore
	metrics      Metrics
	logger       *slog.Logger
	now          func() time.Time
	lastScaleAt  map[types.ServiceID]time.Time
}

type Option func(*Controller)

func WithEventStore(eventStore EventStore) Option {
	return func(c *Controller) {
		c.eventStore = eventStore
	}
}

func WithMetrics(metrics Metrics) Option {
	return func(c *Controller) {
		if metrics != nil {
			c.metrics = metrics
		}
	}
}

func WithClock(now func() time.Time) Option {
	return func(c *Controller) {
		if now != nil {
			c.now = now
		}
	}
}

func NewController(controlPlane ControlPlane, provider MetricsProvider, logger *slog.Logger, opts ...Option) *Controller {
	if logger == nil {
		logger = slog.Default()
	}
	c := &Controller{
		controlPlane: controlPlane,
		provider:     provider,
		metrics:      NoopMetrics{},
		logger:       logger,
		now:          func() time.Time { return time.Now().UTC() },
		lastScaleAt:  map[types.ServiceID]time.Time{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Controller) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := c.RunOnce(ctx); err != nil {
			c.logger.Warn("autoscaler pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Controller) RunOnce(ctx context.Context) error {
	if c.controlPlane == nil {
		return fmt.Errorf("autoscaler control plane is required")
	}
	if c.provider == nil {
		return fmt.Errorf("autoscaler metrics provider is required")
	}
	services, err := c.controlPlane.ListServices(ctx)
	if err != nil {
		c.metrics.IncAutoscalerErrors()
		return fmt.Errorf("list services: %w", err)
	}
	for _, service := range services {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !service.Spec.Autoscaling.Enabled || service.Status != types.ServiceActive {
			continue
		}
		if err := c.reconcileService(ctx, service); err != nil {
			c.metrics.IncAutoscalerErrors()
			c.emit(ctx, service, events.TypeAutoscalerError, types.EventWarning, err.Error(), nil)
			return err
		}
	}
	return nil
}

func (c *Controller) reconcileService(ctx context.Context, service types.Service) error {
	if c.hasActiveRollout(ctx, service.ID) {
		c.metrics.IncAutoscalerDecision("skip_rollout")
		c.emit(ctx, service, events.TypeAutoscalerSkipped, types.EventInfo, "autoscaling skipped during active rollout", map[string]string{"reason": "active_rollout"})
		return nil
	}
	policy := effectivePolicy(service.Spec.Autoscaling)
	if last := c.lastScaleAt[service.ID]; !last.IsZero() && c.now().Sub(last) < policy.Cooldown {
		c.metrics.IncAutoscalerDecision("skip_cooldown")
		c.emit(ctx, service, events.TypeAutoscalerSkipped, types.EventInfo, "autoscaling skipped during cooldown", map[string]string{"reason": "cooldown"})
		return nil
	}
	utilization, err := c.provider.ServiceCPUUtilization(ctx, service)
	if err != nil {
		c.metrics.IncAutoscalerDecision("skip_metrics")
		c.emit(ctx, service, events.TypeAutoscalerSkipped, types.EventWarning, "autoscaling skipped because metrics are unavailable", map[string]string{"reason": "metrics_unavailable"})
		return nil
	}
	recommended := recommendation(service.Spec.Replicas, utilization, policy)
	c.metrics.SetAutoscalerRecommendation(string(service.ID), recommended)
	if recommended == service.Spec.Replicas {
		c.metrics.IncAutoscalerDecision("skip_noop")
		return nil
	}
	updated, err := c.controlPlane.ScaleService(ctx, service.ID, recommended)
	if err != nil {
		return fmt.Errorf("scale service %s to %d replicas: %w", service.ID, recommended, err)
	}
	c.lastScaleAt[service.ID] = c.now()
	decision := "scale_up"
	if recommended < service.Spec.Replicas {
		decision = "scale_down"
	}
	c.metrics.IncAutoscalerDecision(decision)
	c.emit(ctx, updated, events.TypeAutoscalerScaled, types.EventInfo, "autoscaler changed desired replicas", map[string]string{
		"from":            fmt.Sprintf("%d", service.Spec.Replicas),
		"to":              fmt.Sprintf("%d", recommended),
		"cpu_utilization": fmt.Sprintf("%.2f", utilization),
	})
	return nil
}

func (c *Controller) hasActiveRollout(ctx context.Context, serviceID types.ServiceID) bool {
	deployment, err := c.controlPlane.GetServiceRollout(ctx, serviceID)
	if err != nil {
		return false
	}
	return !types.IsTerminalDeploymentStatus(deployment.Status)
}

func (c *Controller) emit(ctx context.Context, service types.Service, eventType string, severity types.EventSeverity, message string, details map[string]string) {
	_ = events.Emit(ctx, c.eventStore, types.Event{
		Type:              eventType,
		Severity:          severity,
		Source:            "autoscaler",
		Message:           message,
		RelatedObjectType: "service",
		RelatedObjectID:   string(service.ID),
		Details:           details,
		Timestamp:         c.now(),
	}, events.WithLogger(c.logger))
}

func effectivePolicy(policy types.AutoscalingPolicy) types.AutoscalingPolicy {
	if policy.Cooldown <= 0 {
		policy.Cooldown = defaultCooldown
	}
	return policy
}

func recommendation(current int, cpuUtilization float64, policy types.AutoscalingPolicy) int {
	if current < policy.MinReplicas {
		current = policy.MinReplicas
	}
	recommended := current
	if policy.TargetCPUUtilization > 0 {
		recommended = int(math.Ceil(float64(current) * cpuUtilization / float64(policy.TargetCPUUtilization)))
	}
	if recommended < policy.MinReplicas {
		recommended = policy.MinReplicas
	}
	if policy.MaxReplicas > 0 && recommended > policy.MaxReplicas {
		recommended = policy.MaxReplicas
	}
	return recommended
}

type FakeMetricsProvider struct {
	mu     sync.Mutex
	values map[types.ServiceID]float64
	err    error
}

func NewFakeMetricsProvider(values map[types.ServiceID]float64) *FakeMetricsProvider {
	cloned := make(map[types.ServiceID]float64, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return &FakeMetricsProvider{values: cloned}
}

func (p *FakeMetricsProvider) Set(serviceID types.ServiceID, value float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.values == nil {
		p.values = map[types.ServiceID]float64{}
	}
	p.values[serviceID] = value
}

func (p *FakeMetricsProvider) SetError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

func (p *FakeMetricsProvider) ServiceCPUUtilization(_ context.Context, service types.Service) (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return 0, p.err
	}
	value, ok := p.values[service.ID]
	if !ok {
		return 0, errors.New("cpu metrics unavailable")
	}
	return value, nil
}

var _ MetricsProvider = (*FakeMetricsProvider)(nil)
