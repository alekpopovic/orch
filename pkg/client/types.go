package client

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

type HealthResponse struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type ErrorResponse struct {
	Error RequestError `json:"error"`
}

type RequestError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type APIError struct {
	StatusCode int
	Status     string
	Code       string
	Message    string
	RequestID  string
	Details    map[string]any
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	code := strings.ReplaceAll(strings.TrimSpace(e.Code), "_", " ")
	message := strings.TrimSpace(e.Message)
	switch {
	case code != "" && message != "":
		message = code + ": " + message
	case code != "":
		message = code
	case message == "":
		message = "server returned " + e.Status
	}
	if details := formatDetails(e.Details); details != "" {
		message += " (" + details + ")"
	}
	if e.RequestID != "" {
		message += " [request_id=" + e.RequestID + "]"
	}
	return message
}

type RolloutServiceRequest struct {
	Image          string `json:"image"`
	MaxUnavailable int    `json:"maxUnavailable"`
	MaxSurge       int    `json:"maxSurge"`
}

type CreateRegistryCredentialRequest struct {
	ID       string `json:"id"`
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type TaskFilter struct {
	ServiceID string
	NodeID    string
	Status    types.TaskStatus
}

func (filter TaskFilter) query() url.Values {
	values := url.Values{}
	setString(values, "service_id", filter.ServiceID)
	setString(values, "node_id", filter.NodeID)
	setString(values, "status", string(filter.Status))
	return values
}

type EventFilter struct {
	ServiceID string
	TaskID    string
	NodeID    string
	Type      string
	Severity  types.EventSeverity
	Since     time.Time
	Limit     int
}

func (filter EventFilter) query() url.Values {
	values := url.Values{}
	setString(values, "service_id", filter.ServiceID)
	setString(values, "task_id", filter.TaskID)
	setString(values, "node_id", filter.NodeID)
	setString(values, "type", filter.Type)
	setString(values, "severity", string(filter.Severity))
	setTime(values, "since", filter.Since)
	setLimit(values, filter.Limit)
	return values
}

type LogStreamRequest struct {
	ServiceID string
	TaskID    string
	Follow    bool
	Tail      string
}

func (request LogStreamRequest) query() url.Values {
	values := url.Values{}
	setString(values, "service_id", request.ServiceID)
	setString(values, "task_id", request.TaskID)
	if request.Follow {
		values.Set("follow", "true")
	}
	setString(values, "tail", request.Tail)
	return values
}

type AuditFilter struct {
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Outcome    string
	Since      time.Time
	Limit      int
}

func (filter AuditFilter) query() url.Values {
	values := url.Values{}
	setString(values, "actor_type", filter.ActorType)
	setString(values, "actor_id", filter.ActorID)
	setString(values, "action", filter.Action)
	setString(values, "target_type", filter.TargetType)
	setString(values, "target_id", filter.TargetID)
	setString(values, "outcome", filter.Outcome)
	setTime(values, "since", filter.Since)
	setLimit(values, filter.Limit)
	return values
}

type AuditLog struct {
	ID         string            `json:"id"`
	Namespace  string            `json:"namespace"`
	ActorType  string            `json:"actor_type"`
	ActorID    string            `json:"actor_id"`
	Action     string            `json:"action"`
	TargetType string            `json:"target_type"`
	TargetID   string            `json:"target_id"`
	RequestID  string            `json:"request_id,omitempty"`
	SourceIP   string            `json:"source_ip,omitempty"`
	Outcome    string            `json:"outcome"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}

type Endpoint struct {
	ServiceID      types.ServiceID    `json:"service_id"`
	ServiceName    string             `json:"service_name"`
	TaskID         types.TaskID       `json:"task_id"`
	NodeID         types.NodeID       `json:"node_id"`
	NodeAddress    string             `json:"node_address"`
	PublicHostPort int                `json:"public_host_port"`
	ContainerPort  int                `json:"container_port"`
	Protocol       types.PortProtocol `json:"protocol"`
	HealthStatus   types.TaskStatus   `json:"health_status"`
	ServiceVersion int64              `json:"service_version"`
}

type ServiceEndpoints struct {
	ServiceID   types.ServiceID `json:"service_id"`
	ServiceName string          `json:"service_name"`
	Endpoints   []Endpoint      `json:"endpoints"`
}

type NodeDrainStatus struct {
	NodeID               types.NodeID     `json:"node_id"`
	NodeStatus           types.NodeStatus `json:"node_status"`
	Phase                string           `json:"phase"`
	TotalTasks           int              `json:"total_tasks"`
	RemainingTasks       int              `json:"remaining_tasks"`
	ReplacementTasks     int              `json:"replacement_tasks"`
	ReplacementReady     int              `json:"replacement_ready"`
	InsufficientCapacity bool             `json:"insufficient_capacity,omitempty"`
	Message              string           `json:"message,omitempty"`
}

type createServiceRequest struct {
	Spec types.ServiceSpec `json:"spec"`
}

type createNamespaceRequest struct {
	Name string `json:"name"`
}

type namespaceResponse struct {
	Namespace types.Namespace `json:"namespace"`
}

type listNamespacesResponse struct {
	Namespaces []types.Namespace `json:"namespaces"`
}

type scaleServiceRequest struct {
	Replicas int `json:"replicas"`
}

type createSecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type nodeResponse struct {
	Node types.Node `json:"node"`
}

type listNodesResponse struct {
	Nodes []types.Node `json:"nodes"`
}

type nodeDrainStatusResponse struct {
	DrainStatus NodeDrainStatus `json:"drain_status"`
}

type serviceResponse struct {
	Service types.Service `json:"service"`
}

type listServicesResponse struct {
	Services []types.Service `json:"services"`
}

type deploymentResponse struct {
	Deployment types.Deployment `json:"deployment"`
}

type listTasksResponse struct {
	Tasks []types.Task `json:"tasks"`
}

type taskResponse struct {
	Task types.Task `json:"task"`
}

type listEventsResponse struct {
	Events []types.Event `json:"events"`
}

type secretResponse struct {
	Secret types.SecretMetadata `json:"secret"`
}

type listSecretsResponse struct {
	Secrets []types.SecretMetadata `json:"secrets"`
}

type registryCredentialResponse struct {
	Credential types.RegistryCredentialMetadata `json:"credential"`
}

type listRegistryCredentialsResponse struct {
	Credentials []types.RegistryCredentialMetadata `json:"credentials"`
}

type listAuditLogsResponse struct {
	AuditLogs []AuditLog `json:"audit_logs"`
}

func formatDetails(details map[string]any) string {
	if len(details) == 0 {
		return ""
	}
	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, details[key]))
	}
	return strings.Join(parts, ", ")
}
