package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/alekpopovic/orch/internal/admission"
	"github.com/alekpopovic/orch/internal/audit"
	"github.com/alekpopovic/orch/internal/policy"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestAdmissionCannotBeBypassedByScaleOrRollout(t *testing.T) {
	service := NewMemoryService(WithClusterPolicy(policy.ClusterPolicy{
		MaxReplicasPerService: 2,
		BlockLatestTag:        true,
	}))
	created, err := service.CreateService(context.Background(), types.ServiceSpec{
		Name: "api", Image: "nginx:1.27", Replicas: 1,
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	if _, err := service.ScaleService(context.Background(), created.ID, 3); err == nil {
		t.Fatal("expected scale admission rejection")
	} else {
		var admissionErr *admission.Error
		if !errors.As(err, &admissionErr) || admissionErr.Violations[0].Rule != "replicas.maximum" {
			t.Fatalf("unexpected scale rejection: %v", err)
		}
	}
	if _, err := service.RolloutService(context.Background(), created.ID, RolloutSpec{
		Image: "nginx:latest", MaxUnavailable: 1, MaxSurge: 1,
	}); err == nil {
		t.Fatal("expected rollout admission rejection")
	}

	logs, err := service.ListAuditLogs(context.Background(), audit.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Outcome != audit.OutcomeFailure || logs[1].Outcome != audit.OutcomeFailure {
		t.Fatalf("expected two rejection audit records, got %#v", logs)
	}
}
