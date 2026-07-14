package autoscaler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestAutoscalerScalesUpAndDown(t *testing.T) {
	ctx := context.Background()
	service := controlplane.NewMemoryService()
	created := createAutoscaledService(t, service, 2, types.AutoscalingPolicy{
		Enabled:              true,
		MinReplicas:          1,
		MaxReplicas:          10,
		TargetCPUUtilization: 50,
		Cooldown:             time.Second,
	})
	provider := NewFakeMetricsProvider(map[types.ServiceID]float64{created.ID: 100})
	metrics := &recordingMetrics{}
	controller := NewController(service, provider, testLogger(), WithEventStore(service), WithMetrics(metrics), WithClock(fixedClock(time.Unix(100, 0))))

	if err := controller.RunOnce(ctx); err != nil {
		t.Fatalf("scale up: %v", err)
	}
	scaled, err := service.GetService(ctx, created.ID)
	if err != nil {
		t.Fatalf("get scaled service: %v", err)
	}
	if scaled.Spec.Replicas != 4 {
		t.Fatalf("expected scale up to 4 replicas, got %d", scaled.Spec.Replicas)
	}

	provider.Set(created.ID, 25)
	controller.now = fixedClock(time.Unix(102, 0))
	if err := controller.RunOnce(ctx); err != nil {
		t.Fatalf("scale down: %v", err)
	}
	scaled, err = service.GetService(ctx, created.ID)
	if err != nil {
		t.Fatalf("get scaled service: %v", err)
	}
	if scaled.Spec.Replicas != 2 {
		t.Fatalf("expected scale down to 2 replicas, got %d", scaled.Spec.Replicas)
	}
	if metrics.decisions["scale_up"] != 1 || metrics.decisions["scale_down"] != 1 {
		t.Fatalf("expected scale decisions, got %#v", metrics.decisions)
	}
	autoscalerEvents, err := service.ListEvents(ctx, events.Filter{Type: events.TypeAutoscalerScaled})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(autoscalerEvents) != 2 {
		t.Fatalf("expected autoscaler scale events, got %#v", autoscalerEvents)
	}
}

func TestAutoscalerRespectsCooldown(t *testing.T) {
	ctx := context.Background()
	service := controlplane.NewMemoryService()
	created := createAutoscaledService(t, service, 2, types.AutoscalingPolicy{
		Enabled:              true,
		MinReplicas:          1,
		MaxReplicas:          10,
		TargetCPUUtilization: 50,
		Cooldown:             time.Minute,
	})
	now := time.Unix(200, 0)
	provider := NewFakeMetricsProvider(map[types.ServiceID]float64{created.ID: 100})
	controller := NewController(service, provider, testLogger(), WithEventStore(service), WithClock(func() time.Time { return now }))

	if err := controller.RunOnce(ctx); err != nil {
		t.Fatalf("first scale: %v", err)
	}
	now = now.Add(30 * time.Second)
	provider.Set(created.ID, 200)
	if err := controller.RunOnce(ctx); err != nil {
		t.Fatalf("cooldown pass: %v", err)
	}
	scaled, err := service.GetService(ctx, created.ID)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if scaled.Spec.Replicas != 4 {
		t.Fatalf("expected cooldown to keep 4 replicas, got %d", scaled.Spec.Replicas)
	}
}

func TestAutoscalerClampsMaxAndMinReplicas(t *testing.T) {
	ctx := context.Background()
	service := controlplane.NewMemoryService()
	maxed := createAutoscaledService(t, service, 5, types.AutoscalingPolicy{
		Enabled:              true,
		MinReplicas:          2,
		MaxReplicas:          6,
		TargetCPUUtilization: 50,
		Cooldown:             time.Second,
	})
	minned := createAutoscaledServiceWithName(t, service, "minned", 4, types.AutoscalingPolicy{
		Enabled:              true,
		MinReplicas:          2,
		MaxReplicas:          10,
		TargetCPUUtilization: 50,
		Cooldown:             time.Second,
	})
	provider := NewFakeMetricsProvider(map[types.ServiceID]float64{
		maxed.ID:  500,
		minned.ID: 1,
	})
	controller := NewController(service, provider, testLogger(), WithClock(fixedClock(time.Unix(300, 0))))

	if err := controller.RunOnce(ctx); err != nil {
		t.Fatalf("run autoscaler: %v", err)
	}
	maxed, err := service.GetService(ctx, maxed.ID)
	if err != nil {
		t.Fatalf("get maxed service: %v", err)
	}
	minned, err = service.GetService(ctx, minned.ID)
	if err != nil {
		t.Fatalf("get minned service: %v", err)
	}
	if maxed.Spec.Replicas != 6 {
		t.Fatalf("expected max clamp to 6, got %d", maxed.Spec.Replicas)
	}
	if minned.Spec.Replicas != 2 {
		t.Fatalf("expected min clamp to 2, got %d", minned.Spec.Replicas)
	}
}

func TestAutoscalerSkipsActiveRollout(t *testing.T) {
	ctx := context.Background()
	service := controlplane.NewMemoryService()
	created := createAutoscaledService(t, service, 2, types.AutoscalingPolicy{
		Enabled:              true,
		MinReplicas:          1,
		MaxReplicas:          10,
		TargetCPUUtilization: 50,
	})
	if _, err := service.RolloutService(ctx, created.ID, controlplane.RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1, MaxSurge: 1}); err != nil {
		t.Fatalf("start rollout: %v", err)
	}
	provider := NewFakeMetricsProvider(map[types.ServiceID]float64{created.ID: 200})
	metrics := &recordingMetrics{}
	controller := NewController(service, provider, testLogger(), WithEventStore(service), WithMetrics(metrics))

	if err := controller.RunOnce(ctx); err != nil {
		t.Fatalf("run autoscaler: %v", err)
	}
	got, err := service.GetService(ctx, created.ID)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if got.Spec.Replicas != 2 {
		t.Fatalf("expected active rollout to keep replicas at 2, got %d", got.Spec.Replicas)
	}
	if metrics.decisions["skip_rollout"] != 1 {
		t.Fatalf("expected skip rollout decision, got %#v", metrics.decisions)
	}
}

func createAutoscaledService(t *testing.T, service *controlplane.MemoryService, replicas int, autoscaling types.AutoscalingPolicy) types.Service {
	t.Helper()
	return createAutoscaledServiceWithName(t, service, "api", replicas, autoscaling)
}

func createAutoscaledServiceWithName(t *testing.T, service *controlplane.MemoryService, name string, replicas int, autoscaling types.AutoscalingPolicy) types.Service {
	t.Helper()
	created, err := service.CreateService(context.Background(), types.ServiceSpec{
		Name:        name,
		Image:       "nginx:1.27",
		Replicas:    replicas,
		Autoscaling: autoscaling,
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return created
}

func fixedClock(now time.Time) func() time.Time {
	return func() time.Time { return now.UTC() }
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type recordingMetrics struct {
	decisions       map[string]int
	errors          int
	recommendations map[string]int
}

func (m *recordingMetrics) IncAutoscalerDecision(decision string) {
	if m.decisions == nil {
		m.decisions = map[string]int{}
	}
	m.decisions[decision]++
}

func (m *recordingMetrics) IncAutoscalerErrors() {
	m.errors++
}

func (m *recordingMetrics) SetAutoscalerRecommendation(serviceID string, replicas int) {
	if m.recommendations == nil {
		m.recommendations = map[string]int{}
	}
	m.recommendations[serviceID] = replicas
}
