package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

func readyTestNode(t *testing.T, s *MemoryService) types.Node {
	t.Helper()
	out, err := s.RegisterNode(context.Background(), NodeRegistration{Name: "worker-1", AdvertiseAddress: "10.0.0.1:7443", Capacity: types.Resources{CPU: 4000, Memory: 8 << 30}, Allocatable: types.Resources{CPU: 4000, Memory: 8 << 30}})
	if err != nil {
		t.Fatal(err)
	}
	return out.Node
}
func jobSpec(name string, limit int) types.JobSpec {
	return types.JobSpec{Name: name, Image: "busybox:1.36", Command: []string{"true"}, BackoffLimit: limit, ResourceRequirements: types.ResourceRequirements{Requests: types.Resources{CPU: 100, Memory: 1 << 20}, Limits: types.Resources{CPU: 100, Memory: 1 << 20}}}
}

func TestJobSuccessFailureRetryAndDelete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryService()
	node := readyTestNode(t, s)
	success, err := s.CreateJob(ctx, jobSpec("success", 0))
	if err != nil {
		t.Fatal(err)
	}
	task := s.tasks[success.TaskIDs[0]]
	task, err = s.AssignTask(ctx, task.ID, node.ID, nil, task.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReportTaskStatus(ctx, TaskStatusReport{TaskID: task.ID, NodeID: node.ID, Status: types.TaskRunning}); err != nil {
		t.Fatal(err)
	}
	zero := 0
	if _, err = s.ReportTaskStatus(ctx, TaskStatusReport{TaskID: task.ID, NodeID: node.ID, Status: types.TaskStopped, ExitCode: &zero}); err != nil {
		t.Fatal(err)
	}
	success, _ = s.GetJob(ctx, success.ID)
	if success.Status != types.JobSucceeded {
		t.Fatalf("status=%s", success.Status)
	}
	failing, err := s.CreateJob(ctx, jobSpec("failure", 1))
	if err != nil {
		t.Fatal(err)
	}
	first := s.tasks[failing.TaskIDs[0]]
	first, err = s.AssignTask(ctx, first.ID, node.ID, nil, first.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	one := 1
	if _, err = s.ReportTaskStatus(ctx, TaskStatusReport{TaskID: first.ID, NodeID: node.ID, Status: types.TaskFailed, ExitCode: &one}); err != nil {
		t.Fatal(err)
	}
	failing, _ = s.GetJob(ctx, failing.ID)
	if failing.Status != types.JobPending || len(failing.TaskIDs) != 2 {
		t.Fatalf("retry not created: %#v", failing)
	}
	second := s.tasks[failing.TaskIDs[1]]
	second, err = s.AssignTask(ctx, second.ID, node.ID, nil, second.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReportTaskStatus(ctx, TaskStatusReport{TaskID: second.ID, NodeID: node.ID, Status: types.TaskFailed, ExitCode: &one}); err != nil {
		t.Fatal(err)
	}
	failing, _ = s.GetJob(ctx, failing.ID)
	if failing.Status != types.JobFailed {
		t.Fatalf("status=%s", failing.Status)
	}
	if err = s.DeleteJob(ctx, failing.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetJob(ctx, failing.ID); err != store.ErrNotFound {
		t.Fatalf("delete err=%v", err)
	}
}

func TestGitOpsManualChangesCreateDriftAndAutoRevert(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryService()
	_ = readyTestNode(t, s)
	source, err := s.CreateGitOpsSource(ctx, types.GitOpsSource{RepositoryURL: "https://example.test/repo.git", Branch: "main", Path: ".", SyncInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.CreateService(ctx, types.ServiceSpec{Name: "api", Image: "nginx:1.27", Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	desired := service.Spec
	if _, err = s.MarkGitOpsManaged(ctx, service.ID, types.GitOpsManagedState{SourceID: source.ID, SourceCommit: "abc", SourcePath: "api.yaml", Policy: types.GitOpsWarnOnly, DesiredSpec: desired}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ScaleService(ctx, service.ID, 3); err != nil {
		t.Fatal(err)
	}
	items, err := s.GitOpsStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].GitOps.Status != types.GitOpsDrifted {
		t.Fatalf("status=%s", items[0].GitOps.Status)
	}
	state := *items[0].GitOps
	state.Policy = types.GitOpsAutoRevert
	if _, err = s.MarkGitOpsManaged(ctx, service.ID, state); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RolloutService(ctx, service.ID, RolloutSpec{Image: "nginx:1.28", MaxUnavailable: 1, MaxSurge: 1}); err != nil {
		t.Fatal(err)
	}
	items, err = s.GitOpsStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Spec.Image != desired.Image || items[0].GitOps.Status != types.GitOpsInSync {
		t.Fatalf("auto-revert failed: %#v", items[0])
	}
}

func TestLocalVolumeSchedulingConflictAndCleanup(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryService()
	node := readyTestNode(t, s)
	volume, err := s.CreateVolume(ctx, types.Volume{Name: "data", Driver: "local", NodeID: node.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateVolumeClaim(ctx, types.VolumeClaim{Name: "data", VolumeID: volume.ID, AccessMode: types.VolumeReadWriteOnce}); err != nil {
		t.Fatal(err)
	}
	service, err := s.CreateService(ctx, types.ServiceSpec{Name: "db", Image: "postgres:17", Replicas: 2, VolumeClaims: []types.VolumeClaimMount{{Claim: "data", Target: "/var/lib/data"}}})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := s.ListTasksByService(ctx, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("RWO should allow one writer, got %d", len(tasks))
	}
	task := tasks[0]
	if len(task.VolumeMounts) != 1 || task.VolumeMounts[0].VolumeID != volume.ID {
		t.Fatal("volume not attached")
	}
	if _, err = s.ReportTaskStatus(ctx, TaskStatusReport{TaskID: task.ID, NodeID: node.ID, Status: types.TaskRemoved}); err != nil {
		t.Fatal(err)
	}
	for _, a := range s.attachments {
		if a.TaskID == task.ID && a.DetachedAt.IsZero() {
			t.Fatal("attachment not cleaned up")
		}
	}
}
