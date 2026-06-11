package docker

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDockerRuntimeIntegration(t *testing.T) {
	if os.Getenv("ORCH_DOCKER_INTEGRATION") != "1" {
		t.Skip("set ORCH_DOCKER_INTEGRATION=1 to run Docker runtime integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runtime, err := NewEngineRuntimeFromEnv()
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	imageName := "busybox:1.36"
	if err := runtime.PullImage(ctx, imageName, nil); err != nil {
		t.Fatalf("pull image: %v", err)
	}

	id, err := runtime.CreateContainer(ctx, ContainerSpec{
		Name:      "orch-integration-task-1",
		Image:     imageName,
		ServiceID: "integration-service",
		TaskID:    "integration-task",
		NodeID:    "integration-node",
		Version:   1,
		Command:   []string{"sh", "-c", "echo hello"},
	})
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	defer func() {
		_ = runtime.RemoveContainer(context.Background(), id, true)
	}()

	again, err := runtime.CreateContainer(ctx, ContainerSpec{
		Name:      "orch-integration-task-1",
		Image:     imageName,
		ServiceID: "integration-service",
		TaskID:    "integration-task",
		NodeID:    "integration-node",
		Version:   1,
		Command:   []string{"sh", "-c", "echo hello"},
	})
	if err != nil {
		t.Fatalf("create existing container: %v", err)
	}
	if again != id {
		t.Fatalf("expected idempotent create to return %q, got %q", id, again)
	}

	if err := runtime.StartContainer(ctx, id); err != nil {
		t.Fatalf("start container: %v", err)
	}
	if err := runtime.StopContainer(ctx, id, time.Second); err != nil {
		t.Fatalf("stop container: %v", err)
	}
	if err := runtime.RemoveContainer(ctx, id, true); err != nil {
		t.Fatalf("remove container: %v", err)
	}
	if err := runtime.RemoveContainer(ctx, id, true); err != nil {
		t.Fatalf("remove container again: %v", err)
	}
}
