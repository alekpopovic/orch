package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestServerMetricsExposeExpectedCollectors(t *testing.T) {
	metrics := NewServer()
	metrics.ObserveAPIRequest(http.MethodGet, "/healthz", http.StatusOK, 10*time.Millisecond)
	metrics.IncSchedulerRuns()
	metrics.IncSchedulerErrors()
	metrics.ObserveSchedulerDuration(20 * time.Millisecond)
	metrics.IncReconciliationRuns()
	metrics.IncReconciliationErrors()
	metrics.ObserveReconciliationDuration(30 * time.Millisecond)
	metrics.AddCreatedTasks(2)
	metrics.IncTasksFailed()
	metrics.IncRollouts()
	metrics.IncRolloutFailures()

	body := scrape(t, metrics.Handler())
	for _, want := range []string{
		"api_requests_total",
		"api_request_duration_seconds",
		"scheduler_runs_total",
		"scheduler_errors_total",
		"scheduler_duration_seconds",
		"reconciler_runs_total",
		"reconciler_errors_total",
		"reconciler_duration_seconds",
		"tasks_created_total 2",
		"tasks_failed_total 1",
		"rollouts_total 1",
		"rollout_failures_total 1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected scrape to contain %q\n%s", want, body)
		}
	}
}

func TestAgentMetricsExposeExpectedCollectors(t *testing.T) {
	metrics := NewAgent()
	metrics.IncHeartbeatSuccess()
	metrics.IncHeartbeatFailure()
	metrics.IncDockerOperation("pull_image")
	metrics.IncDockerOperationError("pull_image")
	metrics.IncTaskStateChange(types.TaskRunning)
	metrics.IncHealthcheckSuccess()
	metrics.IncHealthcheckFailure()

	body := scrape(t, metrics.Handler())
	for _, want := range []string{
		"heartbeat_success_total 1",
		"heartbeat_failure_total 1",
		`docker_operations_total{operation="pull_image"} 1`,
		`docker_operation_errors_total{operation="pull_image"} 1`,
		`task_state_changes_total{status="running"} 1`,
		"healthcheck_success_total 1",
		"healthcheck_failure_total 1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected scrape to contain %q\n%s", want, body)
		}
	}
}

func scrape(t *testing.T, handler http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d", http.StatusOK, rec.Code)
	}
	data, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	return string(data)
}
