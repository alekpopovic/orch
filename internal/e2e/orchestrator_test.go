package e2e

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/agent"
	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/cli"
	"github.com/alekpopovic/orch/internal/config"
	"github.com/alekpopovic/orch/internal/controlplane"
	orchdocker "github.com/alekpopovic/orch/internal/docker"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestOrchestratorMVPE2EWithFakeRuntime(t *testing.T) {
	runOrchestratorMVP(t, orchdocker.NewFakeRuntime())
}

func TestOrchestratorMVPE2EWithRealDocker(t *testing.T) {
	if os.Getenv("ORCH_E2E_DOCKER") != "1" {
		t.Skip("set ORCH_E2E_DOCKER=1 to run real Docker E2E")
	}
	runtime, err := orchdocker.NewEngineRuntimeFromEnv()
	if err != nil {
		t.Fatalf("create Docker runtime: %v", err)
	}
	runOrchestratorMVP(t, runtime)
}

func runOrchestratorMVP(t *testing.T, runtime orchdocker.Runtime) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	controlPlane := controlplane.NewMemoryService()
	handler := api.NewHandler(logger, controlPlane, api.WithBootstrapToken("e2e-registration-token"))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	logServer := httptest.NewServer(agent.NewLogHandler(runtime, "e2e-registration-token", logger))
	t.Cleanup(logServer.Close)

	client, err := cli.NewAPIClient(server.URL)
	if err != nil {
		t.Fatalf("create API client: %v", err)
	}
	agentClient, err := agent.NewHTTPClient(server.URL, "e2e-registration-token")
	if err != nil {
		t.Fatalf("create agent client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runner := agent.NewRunner(config.AgentConfig{
		NodeName:            "e2e-node-1",
		AdvertiseAddress:    logServer.URL,
		AgentAddr:           ":0",
		Labels:              map[string]string{"role": "worker"},
		ServerURL:           server.URL,
		BootstrapToken:      "e2e-registration-token",
		DockerSocketPath:    "/var/run/docker.sock",
		HeartbeatInterval:   20 * time.Millisecond,
		GracefulShutdownTTL: 200 * time.Millisecond,
	}, agentClient, logger).WithRuntime(runtime)
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("agent runner stopped unexpectedly: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("agent runner did not stop")
		}
	})

	var nodeID types.NodeID
	waitFor(t, "agent registration", func() bool {
		nodes, err := client.ListNodes(context.Background())
		if err != nil || len(nodes) != 1 {
			return false
		}
		nodeID = nodes[0].ID
		return nodes[0].Status == types.NodeReady
	})

	service, err := client.CreateService(context.Background(), serviceSpec(2))
	if err != nil {
		t.Fatalf("deploy service: %v", err)
	}
	waitFor(t, "service create event", func() bool {
		items, err := client.ListEvents(context.Background(), events.Filter{ServiceID: service.ID, Type: events.TypeServiceCreated})
		return err == nil && len(items) == 1
	})

	waitFor(t, "two tasks assigned", func() bool {
		tasks := serviceTasks(t, client, service.ID)
		if len(tasks) != 2 {
			return false
		}
		for _, task := range tasks {
			if task.NodeID != nodeID {
				return false
			}
		}
		return true
	})
	waitFor(t, "two tasks running", func() bool {
		return countTasks(serviceTasks(t, client, service.ID), activeRunning) == 2
	})
	var logs bytes.Buffer
	if err := client.StreamLogs(context.Background(), string(service.ID), "", false, "10", &logs); err != nil {
		t.Fatalf("stream logs: %v", err)
	}
	if !strings.Contains(logs.String(), "started fake-container") {
		t.Fatalf("expected task logs to include fake runtime start line, got %q", logs.String())
	}

	if _, err := client.ScaleService(context.Background(), string(service.ID), 3); err != nil {
		t.Fatalf("scale service to 3: %v", err)
	}
	waitFor(t, "three tasks created and running", func() bool {
		tasks := serviceTasks(t, client, service.ID)
		return len(tasks) == 3 && countTasks(tasks, activeRunning) == 3
	})

	if _, err := client.ScaleService(context.Background(), string(service.ID), 1); err != nil {
		t.Fatalf("scale service to 1: %v", err)
	}
	waitFor(t, "extra tasks removed after scale down", func() bool {
		tasks := serviceTasks(t, client, service.ID)
		return len(tasks) == 3 &&
			countTasks(tasks, activeRunning) == 1 &&
			countTasks(tasks, func(task types.Task) bool { return task.ActualStatus == types.TaskRemoved }) == 2
	})

	if err := client.DeleteService(context.Background(), string(service.ID)); err != nil {
		t.Fatalf("delete service: %v", err)
	}
	waitFor(t, "service deletion cleanup", func() bool {
		updated, err := client.GetService(context.Background(), string(service.ID))
		if err != nil {
			return false
		}
		tasks := serviceTasks(t, client, service.ID)
		return updated.Status == types.ServiceDeleted &&
			len(tasks) == 3 &&
			countTasks(tasks, func(task types.Task) bool { return task.ActualStatus == types.TaskRemoved }) == 3
	})
}

func serviceSpec(replicas int) types.ServiceSpec {
	return types.ServiceSpec{
		Name:     "e2e-api",
		Image:    "nginx:1.27-alpine",
		Replicas: replicas,
		ResourceRequirements: types.ResourceRequirements{
			Requests: types.Resources{CPU: 100, Memory: 64 * 1024 * 1024},
			Limits:   types.Resources{CPU: 100, Memory: 64 * 1024 * 1024},
		},
		RestartPolicy: types.RestartPolicy{Condition: types.RestartNever},
		PlacementConstraints: []types.PlacementConstraint{{
			Key:      "role",
			Operator: types.ConstraintEquals,
			Value:    "worker",
		}},
	}
}

func serviceTasks(t *testing.T, client *cli.APIClient, serviceID types.ServiceID) []types.Task {
	t.Helper()
	tasks, err := client.ListTasks(context.Background(), mapValues("service_id", string(serviceID)))
	if err != nil {
		t.Fatalf("list service tasks: %v", err)
	}
	return tasks
}

func mapValues(key string, value string) url.Values {
	return url.Values{key: []string{value}}
}

func activeRunning(task types.Task) bool {
	return task.DesiredStatus == types.TaskRunning && task.ActualStatus == types.TaskRunning
}

func countTasks(tasks []types.Task, match func(types.Task) bool) int {
	count := 0
	for _, task := range tasks {
		if match(task) {
			count++
		}
	}
	return count
}

func waitFor(t *testing.T, name string, condition func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", name)
		case <-ticker.C:
		}
	}
}

func TestPostgresMigrationsE2E(t *testing.T) {
	databaseURL := os.Getenv("ORCH_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ORCH_E2E_DATABASE_URL to run PostgreSQL E2E")
	}
	ctx := context.Background()
	pool, err := store.ConnectPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	downSQL := readMigration(t, "../../migrations/000001_initial_schema.down.sql")
	upSQL := readMigration(t, "../../migrations/000001_initial_schema.up.sql")
	if _, err := pool.Exec(ctx, downSQL); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if _, err := pool.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
}

func readMigration(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}
	return string(data)
}
