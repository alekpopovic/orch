package types

import (
	"errors"
	"testing"
)

func TestValidateNodeTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    NodeStatus
		to      NodeStatus
		wantErr bool
	}{
		{name: "ready to draining", from: NodeReady, to: NodeDraining},
		{name: "draining to ready", from: NodeDraining, to: NodeReady},
		{name: "offline recovery", from: NodeOffline, to: NodeReady},
		{name: "ready shutdown", from: NodeReady, to: NodeOffline},
		{name: "offline cannot drain", from: NodeOffline, to: NodeDraining, wantErr: true},
		{name: "invalid target", from: NodeReady, to: "maintenance", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTransitionError(t, ValidateNodeTransition(tt.from, tt.to), tt.wantErr)
		})
	}
}

func TestValidateServiceTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    ServiceStatus
		to      ServiceStatus
		wantErr bool
	}{
		{name: "active to deleting", from: ServiceActive, to: ServiceDeleting},
		{name: "deleting to deleted", from: ServiceDeleting, to: ServiceDeleted},
		{name: "deleted idempotent", from: ServiceDeleted, to: ServiceDeleted},
		{name: "deleted cannot reactivate", from: ServiceDeleted, to: ServiceActive, wantErr: true},
		{name: "active cannot skip deleted", from: ServiceActive, to: ServiceDeleted, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTransitionError(t, ValidateServiceTransition(tt.from, tt.to), tt.wantErr)
		})
	}
}

func TestValidateTaskTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    TaskStatus
		to      TaskStatus
		wantErr bool
	}{
		{name: "pending to assigned", from: TaskPending, to: TaskAssigned},
		{name: "assigned to pulling", from: TaskAssigned, to: TaskPulling},
		{name: "pulling to created", from: TaskPulling, to: TaskCreated},
		{name: "created to running", from: TaskCreated, to: TaskRunning},
		{name: "running to healthy", from: TaskRunning, to: TaskHealthy},
		{name: "healthy to unhealthy", from: TaskHealthy, to: TaskUnhealthy},
		{name: "unhealthy to failed", from: TaskUnhealthy, to: TaskFailed},
		{name: "running to removed cleanup", from: TaskRunning, to: TaskRemoved},
		{name: "failed to removed recovery", from: TaskFailed, to: TaskRemoved},
		{name: "failed idempotent", from: TaskFailed, to: TaskFailed},
		{name: "failed cannot run", from: TaskFailed, to: TaskRunning, wantErr: true},
		{name: "removed cannot stop", from: TaskRemoved, to: TaskStopped, wantErr: true},
		{name: "pending cannot run", from: TaskPending, to: TaskRunning, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTransitionError(t, ValidateTaskTransition(tt.from, tt.to), tt.wantErr)
		})
	}
}

func TestValidateTaskDesiredTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    TaskStatus
		to      TaskStatus
		wantErr bool
	}{
		{name: "running to stopped", from: TaskRunning, to: TaskStopped},
		{name: "running to removed", from: TaskRunning, to: TaskRemoved},
		{name: "stopped to removed", from: TaskStopped, to: TaskRemoved},
		{name: "stopped idempotent", from: TaskStopped, to: TaskStopped},
		{name: "stopped cannot run", from: TaskStopped, to: TaskRunning, wantErr: true},
		{name: "removed cannot stop", from: TaskRemoved, to: TaskStopped, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTransitionError(t, ValidateTaskDesiredTransition(tt.from, tt.to), tt.wantErr)
		})
	}
}

func TestValidateDeploymentTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    DeploymentStatus
		to      DeploymentStatus
		wantErr bool
	}{
		{name: "pending to running", from: DeploymentPending, to: DeploymentRunning},
		{name: "pending to rolling back", from: DeploymentPending, to: DeploymentRollingBack},
		{name: "running to succeeded", from: DeploymentRunning, to: DeploymentSucceeded},
		{name: "running to failed", from: DeploymentRunning, to: DeploymentFailed},
		{name: "rolling back to rolled back", from: DeploymentRollingBack, to: DeploymentRolledBack},
		{name: "paused to running", from: DeploymentPaused, to: DeploymentRunning},
		{name: "failed idempotent", from: DeploymentFailed, to: DeploymentFailed},
		{name: "succeeded cannot fail", from: DeploymentSucceeded, to: DeploymentFailed, wantErr: true},
		{name: "rolled back cannot run", from: DeploymentRolledBack, to: DeploymentRunning, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTransitionError(t, ValidateDeploymentTransition(tt.from, tt.to), tt.wantErr)
		})
	}
}

func TestAgentCanReportTaskStatus(t *testing.T) {
	tests := []struct {
		name   string
		task   Task
		status TaskStatus
		want   bool
	}{
		{
			name:   "active task accepts progress",
			task:   Task{DesiredStatus: TaskRunning, ActualStatus: TaskAssigned},
			status: TaskRunning,
			want:   true,
		},
		{
			name:   "stopped desired rejects stale running",
			task:   Task{DesiredStatus: TaskStopped, ActualStatus: TaskRunning},
			status: TaskRunning,
		},
		{
			name:   "stopped desired accepts removal",
			task:   Task{DesiredStatus: TaskStopped, ActualStatus: TaskRunning},
			status: TaskRemoved,
			want:   true,
		},
		{
			name:   "terminal duplicate is idempotent",
			task:   Task{DesiredStatus: TaskRunning, ActualStatus: TaskFailed},
			status: TaskFailed,
			want:   true,
		},
		{
			name:   "terminal cannot resurrect",
			task:   Task{DesiredStatus: TaskRunning, ActualStatus: TaskFailed},
			status: TaskRunning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AgentCanReportTaskStatus(tt.task, tt.status); got != tt.want {
				t.Fatalf("AgentCanReportTaskStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func assertTransitionError(t *testing.T, err error, wantErr bool) {
	t.Helper()
	if wantErr {
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("expected ErrInvalidTransition, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("expected transition to be valid, got %v", err)
	}
}
