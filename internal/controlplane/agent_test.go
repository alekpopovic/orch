package controlplane

import (
	"context"
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestAgentHeartbeatStateTransitions(t *testing.T) {
	service := NewMemoryService()
	ctx := context.Background()

	registered, err := service.RegisterNode(ctx, NodeRegistration{
		Name:             "node-a",
		AdvertiseAddress: "10.0.0.10",
		Labels:           map[string]string{"role": "app"},
		Capacity:         types.Resources{CPU: 4000, Memory: 8 * 1024 * 1024 * 1024},
		Allocatable:      types.Resources{CPU: 3500, Memory: 7 * 1024 * 1024 * 1024},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}
	if registered.Node.Status != types.NodeReady {
		t.Fatalf("expected ready after register, got %q", registered.Node.Status)
	}

	drained, err := service.DrainNode(ctx, registered.Node.ID)
	if err != nil {
		t.Fatalf("drain node: %v", err)
	}
	if drained.Status != types.NodeDraining {
		t.Fatalf("expected draining after drain, got %q", drained.Status)
	}

	heartbeat, err := service.HeartbeatNode(ctx, NodeHeartbeat{
		NodeID:      registered.Node.ID,
		Capacity:    registered.Node.Capacity,
		Allocatable: registered.Node.Allocatable,
	})
	if err != nil {
		t.Fatalf("heartbeat node: %v", err)
	}
	if heartbeat.Node.Status != types.NodeDraining {
		t.Fatalf("heartbeat should preserve server draining status, got %q", heartbeat.Node.Status)
	}
	if len(heartbeat.Directives) != 1 || heartbeat.Directives[0].Type != "drain" {
		t.Fatalf("expected drain directive, got %#v", heartbeat.Directives)
	}

	offline, err := service.HeartbeatNode(ctx, NodeHeartbeat{
		NodeID:   registered.Node.ID,
		Shutdown: true,
	})
	if err != nil {
		t.Fatalf("shutdown heartbeat: %v", err)
	}
	if offline.Node.Status != types.NodeOffline {
		t.Fatalf("expected offline after shutdown heartbeat, got %q", offline.Node.Status)
	}
}
