package types

import (
	"errors"
	"fmt"
)

// ErrInvalidTransition marks a rejected lifecycle state transition.
var ErrInvalidTransition = errors.New("invalid state transition")

// ValidNodeStatus reports whether status is a known node lifecycle state.
func ValidNodeStatus(status NodeStatus) bool {
	switch status {
	case NodeReady, NodeDraining, NodeOffline, NodeUnknown:
		return true
	default:
		return false
	}
}

// ValidServiceStatus reports whether status is a known service lifecycle state.
func ValidServiceStatus(status ServiceStatus) bool {
	switch status {
	case ServiceActive, ServiceDeleting, ServiceDeleted:
		return true
	default:
		return false
	}
}

// ValidTaskStatus reports whether status is a known task lifecycle state.
func ValidTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskPending,
		TaskAssigned,
		TaskPulling,
		TaskCreated,
		TaskStarting,
		TaskRunning,
		TaskHealthy,
		TaskUnhealthy,
		TaskStopping,
		TaskStopped,
		TaskRemoved,
		TaskFailed:
		return true
	default:
		return false
	}
}

// ValidDeploymentStatus reports whether status is a known deployment lifecycle state.
func ValidDeploymentStatus(status DeploymentStatus) bool {
	switch status {
	case DeploymentPending,
		DeploymentRunning,
		DeploymentPaused,
		DeploymentSucceeded,
		DeploymentFailed,
		DeploymentRollingBack,
		DeploymentRolledBack:
		return true
	default:
		return false
	}
}

// ValidAgentTaskStatus reports whether an agent may submit status in a task report.
func ValidAgentTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskPulling,
		TaskCreated,
		TaskRunning,
		TaskHealthy,
		TaskUnhealthy,
		TaskFailed,
		TaskStopped,
		TaskRemoved:
		return true
	default:
		return false
	}
}

// ValidateNodeTransition rejects node lifecycle changes outside the documented state machine.
func ValidateNodeTransition(from NodeStatus, to NodeStatus) error {
	if !ValidNodeStatus(from) || !ValidNodeStatus(to) {
		return invalidTransition("node", from, to)
	}
	if from == to {
		return nil
	}
	switch from {
	case NodeUnknown:
		if to == NodeReady || to == NodeOffline {
			return nil
		}
	case NodeReady:
		if to == NodeDraining || to == NodeOffline {
			return nil
		}
	case NodeDraining:
		if to == NodeReady || to == NodeOffline {
			return nil
		}
	case NodeOffline:
		if to == NodeReady {
			return nil
		}
	}
	return invalidTransition("node", from, to)
}

// ValidateServiceTransition rejects service lifecycle changes outside the documented state machine.
func ValidateServiceTransition(from ServiceStatus, to ServiceStatus) error {
	if !ValidServiceStatus(from) || !ValidServiceStatus(to) {
		return invalidTransition("service", from, to)
	}
	if from == to {
		return nil
	}
	switch from {
	case ServiceActive:
		if to == ServiceDeleting {
			return nil
		}
	case ServiceDeleting:
		if to == ServiceDeleted {
			return nil
		}
	}
	return invalidTransition("service", from, to)
}

// ValidateTaskTransition rejects task actual-state changes outside the documented state machine.
func ValidateTaskTransition(from TaskStatus, to TaskStatus) error {
	if !ValidTaskStatus(from) || !ValidTaskStatus(to) {
		return invalidTransition("task", from, to)
	}
	if from == to {
		return nil
	}
	if IsTerminalTaskStatus(from) {
		if to == TaskRemoved && from != TaskRemoved {
			return nil
		}
		return invalidTransition("task", from, to)
	}
	switch from {
	case TaskPending:
		if to == TaskAssigned || to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	case TaskAssigned:
		if to == TaskPulling || to == TaskCreated || to == TaskRunning || to == TaskHealthy || to == TaskUnhealthy || to == TaskStopping || to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	case TaskPulling:
		if to == TaskCreated || to == TaskRunning || to == TaskStopping || to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	case TaskCreated:
		if to == TaskStarting || to == TaskRunning || to == TaskStopping || to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	case TaskStarting:
		if to == TaskRunning || to == TaskStopping || to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	case TaskRunning:
		if to == TaskHealthy || to == TaskUnhealthy || to == TaskStopping || to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	case TaskHealthy:
		if to == TaskRunning || to == TaskUnhealthy || to == TaskStopping || to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	case TaskUnhealthy:
		if to == TaskRunning || to == TaskHealthy || to == TaskStopping || to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	case TaskStopping:
		if to == TaskStopped || to == TaskRemoved || to == TaskFailed {
			return nil
		}
	}
	return invalidTransition("task", from, to)
}

// ValidateTaskDesiredTransition rejects server-owned task desired-state changes outside the documented state machine.
func ValidateTaskDesiredTransition(from TaskStatus, to TaskStatus) error {
	if !ValidTaskStatus(from) || !ValidTaskStatus(to) {
		return invalidTransition("task desired", from, to)
	}
	if from == to {
		return nil
	}
	switch from {
	case TaskRunning:
		if to == TaskStopped || to == TaskRemoved {
			return nil
		}
	case TaskStopped:
		if to == TaskRemoved {
			return nil
		}
	}
	return invalidTransition("task desired", from, to)
}

// ValidateDeploymentTransition rejects rollout lifecycle changes outside the documented state machine.
func ValidateDeploymentTransition(from DeploymentStatus, to DeploymentStatus) error {
	if !ValidDeploymentStatus(from) || !ValidDeploymentStatus(to) {
		return invalidTransition("deployment", from, to)
	}
	if from == to {
		return nil
	}
	if IsTerminalDeploymentStatus(from) {
		return invalidTransition("deployment", from, to)
	}
	switch from {
	case DeploymentPending:
		if to == DeploymentRunning || to == DeploymentRollingBack || to == DeploymentSucceeded || to == DeploymentFailed || to == DeploymentRolledBack {
			return nil
		}
	case DeploymentRunning:
		if to == DeploymentSucceeded || to == DeploymentFailed || to == DeploymentPaused {
			return nil
		}
	case DeploymentPaused:
		if to == DeploymentRunning || to == DeploymentFailed {
			return nil
		}
	case DeploymentRollingBack:
		if to == DeploymentRolledBack || to == DeploymentFailed || to == DeploymentPaused {
			return nil
		}
	}
	return invalidTransition("deployment", from, to)
}

// IsTerminalTaskStatus reports whether a task actual state is final for execution.
// Failed and stopped tasks may still move to removed through documented cleanup recovery.
func IsTerminalTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskStopped, TaskRemoved, TaskFailed:
		return true
	default:
		return false
	}
}

// IsTerminalServiceStatus reports whether a service lifecycle state is immutable.
func IsTerminalServiceStatus(status ServiceStatus) bool {
	return status == ServiceDeleted
}

// IsTerminalDeploymentStatus reports whether a rollout lifecycle state is immutable.
func IsTerminalDeploymentStatus(status DeploymentStatus) bool {
	switch status {
	case DeploymentSucceeded, DeploymentFailed, DeploymentRolledBack:
		return true
	default:
		return false
	}
}

// IsActiveTask reports whether a task still consumes reconciliation and scheduling capacity.
func IsActiveTask(task Task) bool {
	if task.DesiredStatus == TaskStopped || task.DesiredStatus == TaskRemoved {
		return false
	}
	return IsNonTerminalTaskStatus(task.ActualStatus)
}

// IsNonTerminalTaskStatus reports whether a task actual state can still progress.
func IsNonTerminalTaskStatus(status TaskStatus) bool {
	return ValidTaskStatus(status) && !IsTerminalTaskStatus(status)
}

// NonTerminalTaskStatuses returns task actual states that can still progress.
func NonTerminalTaskStatuses() []TaskStatus {
	return []TaskStatus{
		TaskPending,
		TaskAssigned,
		TaskPulling,
		TaskCreated,
		TaskStarting,
		TaskRunning,
		TaskHealthy,
		TaskUnhealthy,
		TaskStopping,
	}
}

// IsAvailableTaskStatus reports whether a task can count toward rollout availability.
func IsAvailableTaskStatus(status TaskStatus) bool {
	return status == TaskRunning || status == TaskHealthy
}

// AgentCanReportTaskStatus applies desired-state and actual-state rules to an agent report.
// Unhealthy is retryable through later healthy/running reports; failed is terminal and replaced by policy.
func AgentCanReportTaskStatus(task Task, status TaskStatus) bool {
	if !ValidAgentTaskStatus(status) {
		return false
	}
	if task.ActualStatus == TaskRemoved {
		return status == TaskRemoved
	}
	if task.DesiredStatus == TaskStopped || task.DesiredStatus == TaskRemoved {
		return status == TaskStopped || status == TaskRemoved || status == TaskFailed
	}
	if IsTerminalTaskStatus(task.ActualStatus) {
		return task.ActualStatus == status
	}
	return ValidateTaskTransition(task.ActualStatus, status) == nil
}

func invalidTransition(kind string, from any, to any) error {
	return fmt.Errorf("%w: %s %q -> %q", ErrInvalidTransition, kind, from, to)
}
