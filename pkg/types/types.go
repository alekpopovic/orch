package types

import (
	"fmt"
	"strings"
	"time"
)

type NodeID string
type TaskID string
type ServiceID string
type DeploymentID string
type EventID string

type Node struct {
	ID               NodeID            `json:"id"`
	Hostname         string            `json:"hostname"`
	AdvertiseAddress string            `json:"advertise_address"`
	Labels           map[string]string `json:"labels,omitempty"`
	Capacity         Resources         `json:"capacity"`
	Allocatable      Resources         `json:"allocatable"`
	Status           NodeStatus        `json:"status"`
	LastHeartbeatAt  time.Time         `json:"last_heartbeat_at"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type Resources struct {
	CPU    int64 `json:"cpu"`
	Memory int64 `json:"memory"`
}

type NodeStatus string

const (
	NodeReady    NodeStatus = "ready"
	NodeDraining NodeStatus = "draining"
	NodeOffline  NodeStatus = "offline"
	NodeUnknown  NodeStatus = "unknown"
)

type NodeSpec struct {
	Hostname         string            `json:"hostname"`
	AdvertiseAddress string            `json:"advertise_address"`
	Labels           map[string]string `json:"labels,omitempty"`
	Capacity         Resources         `json:"capacity"`
	Allocatable      Resources         `json:"allocatable"`
}

func (spec NodeSpec) Validate() error {
	if strings.TrimSpace(spec.Hostname) == "" {
		return fmt.Errorf("hostname is required")
	}
	if strings.TrimSpace(spec.AdvertiseAddress) == "" {
		return fmt.Errorf("advertise address is required")
	}
	if err := validateNonNegativeResources("capacity", spec.Capacity); err != nil {
		return err
	}
	if err := validateNonNegativeResources("allocatable", spec.Allocatable); err != nil {
		return err
	}
	if spec.Allocatable.CPU > spec.Capacity.CPU {
		return fmt.Errorf("allocatable CPU cannot exceed capacity CPU")
	}
	if spec.Allocatable.Memory > spec.Capacity.Memory {
		return fmt.Errorf("allocatable memory cannot exceed capacity memory")
	}
	return nil
}

type Service struct {
	ID                ServiceID     `json:"id"`
	Spec              ServiceSpec   `json:"spec"`
	Status            ServiceStatus `json:"status"`
	DeploymentVersion int64         `json:"deployment_version"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

type ServiceStatus string

const (
	ServiceActive   ServiceStatus = "active"
	ServiceDeleting ServiceStatus = "deleting"
	ServiceDeleted  ServiceStatus = "deleted"
)

type ServiceSpec struct {
	Name                 string                `json:"name"`
	Image                string                `json:"image"`
	Replicas             int                   `json:"replicas"`
	Env                  map[string]string     `json:"env,omitempty"`
	SecretRefs           []SecretRef           `json:"secret_refs,omitempty"`
	Ports                []Port                `json:"ports,omitempty"`
	ResourceRequirements ResourceRequirements  `json:"resource_requirements"`
	Healthcheck          *Healthcheck          `json:"healthcheck,omitempty"`
	RestartPolicy        RestartPolicy         `json:"restart_policy"`
	PlacementConstraints []PlacementConstraint `json:"placement_constraints,omitempty"`
}

func (spec ServiceSpec) Validate() error {
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("service name is required")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return fmt.Errorf("image is required")
	}
	if spec.Replicas < 0 {
		return fmt.Errorf("replicas cannot be negative")
	}
	if err := spec.ResourceRequirements.Validate(); err != nil {
		return err
	}
	for i, port := range spec.Ports {
		if err := port.Validate(); err != nil {
			return fmt.Errorf("ports[%d]: %w", i, err)
		}
	}
	if spec.Healthcheck != nil {
		if err := spec.Healthcheck.Validate(); err != nil {
			return err
		}
	}
	if spec.RestartPolicy.Condition != "" {
		if !validRestartCondition(spec.RestartPolicy.Condition) {
			return fmt.Errorf("restart policy condition %q is invalid", spec.RestartPolicy.Condition)
		}
	}
	for i, constraint := range spec.PlacementConstraints {
		if err := constraint.Validate(); err != nil {
			return fmt.Errorf("placement_constraints[%d]: %w", i, err)
		}
	}
	return nil
}

type SecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

type Port struct {
	Name          string       `json:"name,omitempty"`
	Protocol      PortProtocol `json:"protocol"`
	ContainerPort int          `json:"container_port"`
	PublishedPort int          `json:"published_port,omitempty"`
}

func (port Port) Validate() error {
	if port.Protocol == "" {
		return fmt.Errorf("protocol is required")
	}
	if port.Protocol != PortTCP && port.Protocol != PortUDP {
		return fmt.Errorf("protocol %q is invalid", port.Protocol)
	}
	if port.ContainerPort < 1 || port.ContainerPort > 65535 {
		return fmt.Errorf("container port must be between 1 and 65535")
	}
	if port.PublishedPort < 0 || port.PublishedPort > 65535 {
		return fmt.Errorf("published port must be between 0 and 65535")
	}
	return nil
}

type PortProtocol string

const (
	PortTCP PortProtocol = "tcp"
	PortUDP PortProtocol = "udp"
)

type ResourceRequirements struct {
	Requests Resources `json:"requests"`
	Limits   Resources `json:"limits"`
}

func (requirements ResourceRequirements) Validate() error {
	if err := validateNonNegativeResources("resource requests", requirements.Requests); err != nil {
		return err
	}
	if err := validateNonNegativeResources("resource limits", requirements.Limits); err != nil {
		return err
	}
	if requirements.Limits.CPU > 0 && requirements.Requests.CPU > requirements.Limits.CPU {
		return fmt.Errorf("requested CPU cannot exceed CPU limit")
	}
	if requirements.Limits.Memory > 0 && requirements.Requests.Memory > requirements.Limits.Memory {
		return fmt.Errorf("requested memory cannot exceed memory limit")
	}
	return nil
}

type Healthcheck struct {
	Type               HealthcheckType `json:"type"`
	Path               string          `json:"path,omitempty"`
	Port               int             `json:"port"`
	Interval           time.Duration   `json:"interval"`
	Timeout            time.Duration   `json:"timeout"`
	HealthyThreshold   int             `json:"healthy_threshold"`
	UnhealthyThreshold int             `json:"unhealthy_threshold"`
}

func (check Healthcheck) Validate() error {
	if check.Type == HealthcheckNone {
		return nil
	}
	if check.Type != HealthcheckHTTP && check.Type != HealthcheckTCP {
		return fmt.Errorf("healthcheck type %q is invalid", check.Type)
	}
	if check.Port < 1 || check.Port > 65535 {
		return fmt.Errorf("healthcheck port must be between 1 and 65535")
	}
	if check.Type == HealthcheckHTTP && strings.TrimSpace(check.Path) == "" {
		return fmt.Errorf("healthcheck path is required for HTTP checks")
	}
	if check.Interval < 0 {
		return fmt.Errorf("healthcheck interval cannot be negative")
	}
	if check.Timeout < 0 {
		return fmt.Errorf("healthcheck timeout cannot be negative")
	}
	if check.HealthyThreshold < 0 {
		return fmt.Errorf("healthy threshold cannot be negative")
	}
	if check.UnhealthyThreshold < 0 {
		return fmt.Errorf("unhealthy threshold cannot be negative")
	}
	return nil
}

type HealthcheckType string

const (
	HealthcheckNone HealthcheckType = "none"
	HealthcheckHTTP HealthcheckType = "http"
	HealthcheckTCP  HealthcheckType = "tcp"
)

type RestartPolicy struct {
	Condition   RestartCondition `json:"condition"`
	MaxAttempts int              `json:"max_attempts,omitempty"`
}

type RestartCondition string

const (
	RestartNever     RestartCondition = "never"
	RestartOnFailure RestartCondition = "on_failure"
	RestartAlways    RestartCondition = "always"
)

type PlacementConstraint struct {
	Key      string             `json:"key"`
	Operator ConstraintOperator `json:"operator"`
	Value    string             `json:"value"`
}

func (constraint PlacementConstraint) Validate() error {
	if strings.TrimSpace(constraint.Key) == "" {
		return fmt.Errorf("key is required")
	}
	if constraint.Operator != ConstraintEquals && constraint.Operator != ConstraintNotEquals {
		return fmt.Errorf("operator %q is invalid", constraint.Operator)
	}
	return nil
}

type ConstraintOperator string

const (
	ConstraintEquals    ConstraintOperator = "equals"
	ConstraintNotEquals ConstraintOperator = "not_equals"
)

type Task struct {
	ID            TaskID     `json:"id"`
	ServiceID     ServiceID  `json:"service_id"`
	NodeID        NodeID     `json:"node_id,omitempty"`
	ContainerID   string     `json:"container_id,omitempty"`
	DesiredStatus TaskStatus `json:"desired_status"`
	ActualStatus  TaskStatus `json:"actual_status"`
	Image         string     `json:"image"`
	Version       int64      `json:"version"`
	RestartCount  int        `json:"restart_count"`
	FailureReason string     `json:"failure_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	StartedAt     time.Time  `json:"started_at,omitempty"`
	FinishedAt    time.Time  `json:"finished_at,omitempty"`
}

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskAssigned  TaskStatus = "assigned"
	TaskPulling   TaskStatus = "pulling"
	TaskCreated   TaskStatus = "created"
	TaskStarting  TaskStatus = "starting"
	TaskRunning   TaskStatus = "running"
	TaskHealthy   TaskStatus = "healthy"
	TaskUnhealthy TaskStatus = "unhealthy"
	TaskStopping  TaskStatus = "stopping"
	TaskStopped   TaskStatus = "stopped"
	TaskRemoved   TaskStatus = "removed"
	TaskFailed    TaskStatus = "failed"
)

type Deployment struct {
	ID             DeploymentID     `json:"id"`
	ServiceID      ServiceID        `json:"service_id"`
	FromVersion    int64            `json:"from_version"`
	ToVersion      int64            `json:"to_version"`
	Strategy       RolloutStrategy  `json:"strategy"`
	Status         DeploymentStatus `json:"status"`
	MaxUnavailable int              `json:"max_unavailable"`
	MaxSurge       int              `json:"max_surge"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	StartedAt      time.Time        `json:"started_at,omitempty"`
	CompletedAt    time.Time        `json:"completed_at,omitempty"`
}

type RolloutStrategy string

const (
	RolloutRollingUpdate RolloutStrategy = "rolling_update"
	RolloutRecreate      RolloutStrategy = "recreate"
)

type DeploymentStatus string

const (
	DeploymentPending     DeploymentStatus = "pending"
	DeploymentRunning     DeploymentStatus = "running"
	DeploymentPaused      DeploymentStatus = "paused"
	DeploymentSucceeded   DeploymentStatus = "succeeded"
	DeploymentFailed      DeploymentStatus = "failed"
	DeploymentRollingBack DeploymentStatus = "rolling_back"
	DeploymentRolledBack  DeploymentStatus = "rolled_back"
)

type Event struct {
	ID                EventID       `json:"id"`
	Type              string        `json:"type"`
	Severity          EventSeverity `json:"severity"`
	Source            string        `json:"source"`
	Message           string        `json:"message"`
	RelatedObjectType string        `json:"related_object_type,omitempty"`
	RelatedObjectID   string        `json:"related_object_id,omitempty"`
	Timestamp         time.Time     `json:"timestamp"`
}

type EventSeverity string

const (
	EventInfo    EventSeverity = "info"
	EventWarning EventSeverity = "warning"
	EventWarn    EventSeverity = EventWarning
	EventError   EventSeverity = "error"
)

func validateNonNegativeResources(name string, resources Resources) error {
	if resources.CPU < 0 {
		return fmt.Errorf("%s CPU cannot be negative", name)
	}
	if resources.Memory < 0 {
		return fmt.Errorf("%s memory cannot be negative", name)
	}
	return nil
}

func validRestartCondition(condition RestartCondition) bool {
	switch condition {
	case RestartNever, RestartOnFailure, RestartAlways:
		return true
	default:
		return false
	}
}
