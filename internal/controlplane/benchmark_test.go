package controlplane

import (
	"context"
	"fmt"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
	"testing"
)

func BenchmarkEventInsertion(b *testing.B) {
	s := NewMemoryService()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.AppendEvent(ctx, types.Event{Type: "benchmark", Message: "event"})
	}
}
func BenchmarkServiceAndTaskLists(b *testing.B) {
	s := NewMemoryService()
	ctx := context.Background()
	_, _ = s.RegisterNode(ctx, NodeRegistration{Name: "node", AdvertiseAddress: "10.0.0.1", Capacity: types.Resources{CPU: 100000, Memory: 1 << 50}, Allocatable: types.Resources{CPU: 100000, Memory: 1 << 50}})
	for i := 0; i < 1000; i++ {
		_, _ = s.CreateService(ctx, types.ServiceSpec{Name: fmt.Sprintf("svc-%04d", i), Image: "nginx:1.27", Replicas: 1})
	}
	b.Run("services", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = s.ListServices(ctx)
		}
	})
	b.Run("tasks", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = s.ListTasks(ctx, TaskFilter{})
		}
	})
	b.Run("events", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = s.ListEvents(ctx, events.Filter{Limit: 10000})
		}
	})
}
func BenchmarkReconciliationManyServices(b *testing.B) {
	for n := 0; n < b.N; n++ {
		s := NewMemoryService()
		ctx := context.Background()
		_, _ = s.RegisterNode(ctx, NodeRegistration{Name: "node", AdvertiseAddress: "10.0.0.1", Capacity: types.Resources{CPU: 100000, Memory: 1 << 50}, Allocatable: types.Resources{CPU: 100000, Memory: 1 << 50}})
		for i := 0; i < 1000; i++ {
			_, _ = s.CreateService(ctx, types.ServiceSpec{Name: fmt.Sprintf("svc-%04d", i), Image: "nginx:1.27", Replicas: 1})
		}
	}
}
