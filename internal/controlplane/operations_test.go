package controlplane

import (
	"context"
	"github.com/alekpopovic/orch/internal/audit"
	"github.com/alekpopovic/orch/internal/maintenance"
	"github.com/alekpopovic/orch/pkg/types"
	"testing"
	"time"
)

func TestMaintenanceWindowAndForcedBypass(t *testing.T) {
	s := NewMemoryService()
	now := time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	ctx := context.Background()
	_, err := s.CreateMaintenanceWindow(ctx, types.MaintenanceWindow{Name: "rollouts", Schedule: "0 2 * * *", Timezone: "UTC", Duration: time.Hour, Enabled: true, AllowedOperations: []types.MaintenanceOperation{types.MaintenanceRollout}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.CreateService(ctx, types.ServiceSpec{Name: "api", Image: "nginx:1.27", Replicas: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RolloutService(ctx, service.ID, RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1}); err == nil {
		t.Fatal("outside-window rollout allowed")
	}
	if _, err = s.RolloutService(maintenance.WithForce(ctx), service.ID, RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1}); err != nil {
		t.Fatal(err)
	}
	logs, err := s.ListAuditLogs(ctx, audit.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, log := range logs {
		if log.Action == "maintenance.bypass" {
			found = true
		}
	}
	if !found {
		t.Fatal("forced bypass not audited")
	}
}
func TestRetentionPruneAndDryRun(t *testing.T) {
	s := NewMemoryService()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	old := now.Add(-100 * 24 * time.Hour)
	s.events = []types.Event{{ID: "old", Timestamp: old}, {ID: "recent", Timestamp: now}}
	s.tasks["active"] = types.Task{ID: "active", ActualStatus: types.TaskRunning, UpdatedAt: old}
	s.tasks["unresolved"] = types.Task{ID: "unresolved", ServiceID: "missing", Version: 1, DesiredStatus: types.TaskRunning, ActualStatus: types.TaskFailed, FinishedAt: old, UpdatedAt: old}
	s.tasks["resolved"] = types.Task{ID: "resolved", ServiceID: "replaced", Version: 1, DesiredStatus: types.TaskRunning, ActualStatus: types.TaskFailed, FinishedAt: old, UpdatedAt: old}
	s.tasks["replacement"] = types.Task{ID: "replacement", ServiceID: "replaced", Version: 1, DesiredStatus: types.TaskRunning, ActualStatus: types.TaskRunning, UpdatedAt: now}
	result, err := s.PruneRetention(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Events != 1 || len(s.events) != 2 {
		t.Fatalf("dry run mutated data: %#v", result)
	}
	result, err = s.PruneRetention(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Events != 1 || len(s.events) != 1 || s.events[0].ID != "recent" {
		t.Fatalf("prune result %#v events %#v", result, s.events)
	}
	if _, ok := s.tasks["active"]; !ok {
		t.Fatal("active task pruned")
	}
	if _, ok := s.tasks["unresolved"]; !ok {
		t.Fatal("unresolved failed task pruned")
	}
	if _, ok := s.tasks["resolved"]; ok {
		t.Fatal("resolved failed task was not pruned")
	}
}
func TestUsageSnapshotAggregation(t *testing.T) {
	s := NewMemoryService()
	ctx := context.Background()
	_, err := s.CreateService(ctx, types.ServiceSpec{Name: "api", Image: "nginx:1.27", Replicas: 2, Ports: []types.Port{{Protocol: types.PortTCP, ContainerPort: 80, PublishedPort: 8080}}, ResourceRequirements: types.ResourceRequirements{Requests: types.Resources{CPU: 250, Memory: 128 << 20}, Limits: types.Resources{CPU: 250, Memory: 128 << 20}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateVolumeClaim(ctx, types.VolumeClaim{Name: "data", AccessMode: types.VolumeReadWriteOnce}); err != nil {
		t.Fatal(err)
	}
	snapshots, err := s.CaptureUsageSnapshots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].CPUMillicores != 500 || snapshots[0].StorageClaims != 1 {
		t.Fatalf("snapshot %#v", snapshots)
	}
	report, err := s.GetUsageReport(ctx, "default", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.Services != 1 || report.Totals.Replicas != 2 {
		t.Fatalf("report %#v", report)
	}
}
