package gitops_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/audit"
	"github.com/alekpopovic/orch/internal/cli"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/gitops"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestControllerSyncsLocalFixtureRepositoryAndPrunes(t *testing.T) {
	repository := t.TempDir()
	runGit(t, repository, "init", "-b", "main")
	runGit(t, repository, "config", "user.email", "gitops-test@example.com")
	runGit(t, repository, "config", "user.name", "GitOps Test")
	manifest := filepath.Join(repository, "api.yaml")
	if err := os.WriteFile(manifest, []byte("name: api\nimage: nginx:1.27\nreplicas: 1\nresources:\n  cpu: 100m\n  memory: 64Mi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "api.yaml")
	runGit(t, repository, "commit", "-m", "add api")

	service := controlplane.NewMemoryService()
	source, err := service.CreateGitOpsSource(context.Background(), types.GitOpsSource{
		RepositoryURL: repository, Branch: "main", Path: ".", SyncInterval: time.Minute, Prune: true,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	controller := gitops.NewController(service, nil, cli.ParseDeploy, nil)
	synced, err := controller.Sync(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if synced.LastRevision == "" || len(synced.ManagedServices) != 1 || synced.ManagedServices[0] != "api" {
		t.Fatalf("unexpected sync status: %#v", synced)
	}
	eventItems, err := service.ListEvents(context.Background(), events.Filter{Type: gitops.EventSyncSucceeded})
	if err != nil || len(eventItems) != 1 {
		t.Fatalf("expected GitOps sync event, events=%#v err=%v", eventItems, err)
	}
	auditItems, err := service.ListAuditLogs(context.Background(), audit.Filter{Action: "gitops.sync"})
	if err != nil || len(auditItems) != 1 || auditItems[0].Outcome != audit.OutcomeSuccess {
		t.Fatalf("expected GitOps sync audit, logs=%#v err=%v", auditItems, err)
	}
	services, err := service.ListServices(context.Background())
	if err != nil || len(services) != 1 || services[0].Spec.Name != "api" {
		t.Fatalf("service was not applied: services=%#v err=%v", services, err)
	}
	if _, err := controller.Sync(context.Background(), source.ID); err != nil {
		t.Fatalf("idempotent sync: %v", err)
	}
	afterRepeat, err := service.GetService(context.Background(), services[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRepeat.DeploymentVersion != services[0].DeploymentVersion {
		t.Fatalf("unchanged Git revision should not create a service version: before=%d after=%d", services[0].DeploymentVersion, afterRepeat.DeploymentVersion)
	}

	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "-A")
	runGit(t, repository, "commit", "-m", "remove api")
	if _, err := controller.Sync(context.Background(), source.ID); err != nil {
		t.Fatalf("prune sync: %v", err)
	}
	deleted, err := service.GetService(context.Background(), services[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != types.ServiceDeleted {
		t.Fatalf("expected pruned service to be deleted, got %s", deleted.Status)
	}
}

func TestSourceRejectsPlaintextCredentials(t *testing.T) {
	err := gitops.ValidateSource(types.GitOpsSource{
		RepositoryURL: "https://user:token@example.com/repo.git", Branch: "main", Path: ".", SyncInterval: time.Minute,
	})
	if err == nil {
		t.Fatal("expected repository URL credentials to be rejected")
	}
}

func TestControllerRunStopsOnContextCancellation(t *testing.T) {
	service := controlplane.NewMemoryService()
	controller := gitops.NewController(service, nil, cli.ParseDeploy, nil, gitops.WithResolution(5*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := controller.Run(ctx); err == nil {
		t.Fatal("expected context cancellation")
	}
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
