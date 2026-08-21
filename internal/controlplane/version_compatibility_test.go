package controlplane

import (
	"context"
	"strings"
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestAgentVersionCompatibility(t *testing.T) {
	service := NewMemoryService()
	registration := NodeRegistration{
		Name:             "worker-versioned",
		AdvertiseAddress: "10.0.0.8:7443",
		Capacity:         types.Resources{CPU: 1000, Memory: 1 << 30},
		Allocatable:      types.Resources{CPU: 1000, Memory: 1 << 30},
	}

	registration.AgentVersion = "0.1.9"
	if _, err := service.RegisterNode(context.Background(), registration); err == nil {
		t.Fatal("expected an agent older than the compatibility floor to be rejected")
	}

	registration.AgentVersion = "0.4.0"
	command, err := service.RegisterNode(context.Background(), registration)
	if err != nil {
		t.Fatal(err)
	}
	if command.Node.AgentVersion != "0.4.0" {
		t.Fatalf("agent version was not recorded: %#v", command.Node)
	}
	foundWarning := false
	for _, directive := range command.Directives {
		foundWarning = foundWarning || directive.Type == "version_warning" || strings.Contains(directive.Message, "newer")
	}
	if !foundWarning {
		t.Fatalf("expected warning directive for untested agent: %#v", command.Directives)
	}
}
