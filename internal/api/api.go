package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/admission"
	"github.com/alekpopovic/orch/internal/apperrors"
	"github.com/alekpopovic/orch/internal/audit"
	"github.com/alekpopovic/orch/internal/auth"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/discovery"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/namespace"
	"github.com/alekpopovic/orch/internal/quota"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/internal/traefik"
	versioninfo "github.com/alekpopovic/orch/internal/version"
	"github.com/alekpopovic/orch/pkg/types"
)

type Server struct {
	controlPlane   controlplane.Service
	logStreamer    LogStreamer
	logger         *slog.Logger
	timeout        time.Duration
	bootstrapToken string
	jwtSecret      string
	staticUsers    map[string]auth.Role
	agentIssuer    auth.AgentCredentialIssuer
	metrics        RequestMetrics
	controlMetrics ControlMetrics
	metricsHandler http.Handler
	auditStore     audit.Store
	now            func() time.Time
	gitopsSyncer   GitOpsSyncer
}

type GitOpsSyncer interface {
	Sync(context.Context, string) (types.GitOpsSource, error)
}

type Option func(*Server)

type RequestMetrics interface {
	ObserveAPIRequest(method string, route string, status int, duration time.Duration)
}

type ControlMetrics interface {
	IncTasksFailed()
	IncRollouts()
}

func WithTimeout(timeout time.Duration) Option {
	return func(server *Server) {
		server.timeout = timeout
	}
}

func WithBootstrapToken(token string) Option {
	return func(server *Server) {
		server.bootstrapToken = token
	}
}

func WithUserJWT(secret string) Option {
	return func(server *Server) {
		server.jwtSecret = secret
	}
}

func WithStaticUsers(users map[string]auth.Role) Option {
	return func(server *Server) {
		server.staticUsers = users
	}
}

func WithLogStreamer(streamer LogStreamer) Option {
	return func(server *Server) {
		server.logStreamer = streamer
	}
}

func WithAgentCredentialIssuer(issuer auth.AgentCredentialIssuer) Option {
	return func(server *Server) {
		server.agentIssuer = issuer
	}
}

func WithRequestMetrics(metrics RequestMetrics) Option {
	return func(server *Server) {
		server.metrics = metrics
	}
}

func WithControlMetrics(metrics ControlMetrics) Option {
	return func(server *Server) {
		server.controlMetrics = metrics
	}
}

func WithMetricsHandler(handler http.Handler) Option {
	return func(server *Server) {
		server.metricsHandler = handler
	}
}

func WithAuditStore(store audit.Store) Option {
	return func(server *Server) {
		server.auditStore = store
	}
}

func WithGitOpsSyncer(syncer GitOpsSyncer) Option {
	return func(server *Server) { server.gitopsSyncer = syncer }
}

func NewHandler(logger *slog.Logger, controlPlane controlplane.Service, opts ...Option) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if controlPlane == nil {
		controlPlane = controlplane.NewMemoryService()
	}
	server := &Server{
		controlPlane: controlPlane,
		logger:       logger,
		timeout:      15 * time.Second,
		now:          func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(server)
	}
	if server.auditStore == nil {
		if store, ok := controlPlane.(audit.Store); ok {
			server.auditStore = store
		}
	}
	if server.logStreamer == nil {
		server.logStreamer = &AgentHTTPLogStreamer{}
	}
	if server.agentIssuer == nil {
		server.agentIssuer = auth.NewTokenAgentCredentialIssuer(15 * time.Minute)
	}
	if server.metricsHandler == nil {
		server.metricsHandler = http.NotFoundHandler()
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", server.metricsHandler)
	mux.HandleFunc("GET /healthz", server.healthz)
	mux.HandleFunc("GET /readyz", server.readyz)
	server.registerV1(mux)

	return server.middleware(mux)
}

func (s *Server) registerV1(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/version", s.apiVersion)
	mux.HandleFunc("POST /v1/agent/register", s.agentRegister)
	mux.HandleFunc("POST /v1/agent/heartbeat", s.agentHeartbeat)
	mux.HandleFunc("GET /v1/agent/tasks", s.agentListTasks)
	mux.HandleFunc("POST /v1/agent/tasks/{task_id}/status", s.agentTaskStatus)
	mux.HandleFunc("GET /v1/nodes", s.listNodes)
	mux.HandleFunc("GET /v1/nodes/{id}", s.getNode)
	mux.HandleFunc("POST /v1/nodes/{id}/drain", s.drainNode)
	mux.HandleFunc("POST /v1/nodes/{id}/uncordon", s.uncordonNode)
	mux.HandleFunc("GET /v1/nodes/{id}/drain-status", s.getNodeDrainStatus)
	mux.HandleFunc("POST /v1/secrets", s.createSecret)
	mux.HandleFunc("GET /v1/secrets", s.listSecrets)
	mux.HandleFunc("GET /v1/secrets/{name...}", s.getSecret)
	mux.HandleFunc("DELETE /v1/secrets/{name...}", s.deleteSecret)
	mux.HandleFunc("POST /v1/registry-credentials", s.createRegistryCredential)
	mux.HandleFunc("GET /v1/registry-credentials", s.listRegistryCredentials)
	mux.HandleFunc("DELETE /v1/registry-credentials/{id}", s.deleteRegistryCredential)
	mux.HandleFunc("POST /v1/services", s.createService)
	mux.HandleFunc("GET /v1/services", s.listServices)
	mux.HandleFunc("GET /v1/services/{id}", s.getService)
	mux.HandleFunc("GET /v1/services/{id}/endpoints", s.getServiceEndpoints)
	mux.HandleFunc("DELETE /v1/services/{id}", s.deleteService)
	mux.HandleFunc("POST /v1/services/{id}/scale", s.scaleService)
	mux.HandleFunc("POST /v1/services/{id}/rollout", s.rolloutService)
	mux.HandleFunc("GET /v1/services/{id}/rollout", s.getServiceRollout)
	mux.HandleFunc("GET /v1/rollouts/{id}", s.getRollout)
	mux.HandleFunc("POST /v1/services/{id}/rollback", s.rollbackService)
	mux.HandleFunc("GET /v1/tasks", s.listTasks)
	mux.HandleFunc("GET /v1/tasks/{id}", s.getTask)
	mux.HandleFunc("GET /v1/discovery/services", s.discoveryServices)
	mux.HandleFunc("GET /v1/discovery/services/{name}", s.discoveryServiceByName)
	mux.HandleFunc("GET /v1/integrations/traefik/config", s.traefikConfig)
	mux.HandleFunc("GET /v1/events", s.listEvents)
	mux.HandleFunc("GET /v1/audit", s.listAuditLogs)
	mux.HandleFunc("GET /v1/logs", s.streamLogs)
	mux.HandleFunc("POST /v1/namespaces", s.createNamespace)
	mux.HandleFunc("GET /v1/namespaces", s.listNamespaces)
	mux.HandleFunc("DELETE /v1/namespaces/{name}", s.deleteNamespace)
	mux.HandleFunc("GET /v1/quota", s.getResourceQuota)
	mux.HandleFunc("PUT /v1/quota", s.setResourceQuota)
	mux.HandleFunc("POST /v1/gitops/sources", s.createGitOpsSource)
	mux.HandleFunc("GET /v1/gitops/sources", s.listGitOpsSources)
	mux.HandleFunc("DELETE /v1/gitops/sources/{id}", s.deleteGitOpsSource)
	mux.HandleFunc("POST /v1/gitops/sources/{id}/sync", s.syncGitOpsSource)
	mux.HandleFunc("GET /v1/gitops/status", s.gitopsStatus)
	mux.HandleFunc("GET /v1/gitops/diff/{service}", s.gitopsDiff)
	mux.HandleFunc("POST /v1/jobs", s.createJob)
	mux.HandleFunc("GET /v1/jobs", s.listJobs)
	mux.HandleFunc("GET /v1/jobs/{id}", s.getJob)
	mux.HandleFunc("DELETE /v1/jobs/{id}", s.deleteJob)
	mux.HandleFunc("POST /v1/cronjobs", s.createCronJob)
	mux.HandleFunc("GET /v1/cronjobs", s.listCronJobs)
	mux.HandleFunc("GET /v1/cronjobs/{id}", s.getCronJob)
	mux.HandleFunc("DELETE /v1/cronjobs/{id}", s.deleteCronJob)
	mux.HandleFunc("POST /v1/cronjobs/{id}/suspend", s.suspendCronJob)
	mux.HandleFunc("POST /v1/cronjobs/{id}/resume", s.resumeCronJob)
	mux.HandleFunc("POST /v1/volumes", s.createVolume)
	mux.HandleFunc("GET /v1/volumes", s.listVolumes)
	mux.HandleFunc("GET /v1/volumes/{id}", s.getVolume)
	mux.HandleFunc("DELETE /v1/volumes/{id}", s.deleteVolume)
	mux.HandleFunc("POST /v1/volume-claims", s.createVolumeClaim)
	mux.HandleFunc("GET /v1/volume-claims", s.listVolumeClaims)
	mux.HandleFunc("POST /v1/notification-sinks", s.createNotificationSink)
	mux.HandleFunc("GET /v1/notification-sinks", s.listNotificationSinks)
	mux.HandleFunc("DELETE /v1/notification-sinks/{id}", s.deleteNotificationSink)
	mux.HandleFunc("POST /v1/notification-sinks/{id}/test", s.testNotificationSink)
	mux.HandleFunc("POST /v1/maintenance-windows", s.createMaintenanceWindow)
	mux.HandleFunc("GET /v1/maintenance-windows", s.listMaintenanceWindows)
	mux.HandleFunc("DELETE /v1/maintenance-windows/{id}", s.deleteMaintenanceWindow)
	mux.HandleFunc("GET /v1/retention", s.retentionStatus)
	mux.HandleFunc("POST /v1/retention/prune", s.pruneRetention)
	mux.HandleFunc("GET /v1/usage", s.getUsage)
}

type HealthResponse struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type VersionResponse struct {
	APIVersion                string `json:"api_version"`
	ServerVersion             string `json:"server_version"`
	MinimumAgentVersion       string `json:"minimum_agent_version"`
	MaximumTestedAgentVersion string `json:"maximum_tested_agent_version"`
	DatabaseSchemaVersion     int    `json:"database_schema_version"`
	MinimumSchemaVersion      int    `json:"minimum_schema_version"`
	MaximumSchemaVersion      int    `json:"maximum_schema_version"`
}

type CreateNamespaceRequest struct {
	Name string `json:"name"`
}

type NamespaceResponse struct {
	Namespace types.Namespace `json:"namespace"`
}

type ListNamespacesResponse struct {
	Namespaces []types.Namespace `json:"namespaces"`
}

type ResourceQuotaResponse struct {
	Quota types.ResourceQuota `json:"quota"`
	Usage types.ResourceUsage `json:"usage"`
}

type CreateGitOpsSourceRequest struct {
	RepositoryURL string                  `json:"repository_url"`
	Branch        string                  `json:"branch"`
	Path          string                  `json:"path"`
	SyncInterval  string                  `json:"sync_interval"`
	Prune         bool                    `json:"prune"`
	DriftPolicy   types.GitOpsDriftPolicy `json:"drift_policy,omitempty"`
}

type GitOpsSourceResponse struct {
	Source types.GitOpsSource `json:"source"`
}

type ListGitOpsSourcesResponse struct {
	Sources []types.GitOpsSource `json:"sources"`
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

type ListNodesResponse struct {
	Nodes []types.Node `json:"nodes"`
}

type AgentRegisterRequest struct {
	NodeName         string            `json:"node_name"`
	AdvertiseAddress string            `json:"advertise_address"`
	Labels           map[string]string `json:"labels,omitempty"`
	Capacity         types.Resources   `json:"capacity"`
	Allocatable      types.Resources   `json:"allocatable"`
	DockerSocketPath string            `json:"docker_socket_path,omitempty"`
	AgentVersion     string            `json:"agent_version"`
}

type AgentHeartbeatRequest struct {
	NodeID       types.NodeID      `json:"node_id"`
	Capacity     types.Resources   `json:"capacity"`
	Allocatable  types.Resources   `json:"allocatable"`
	Labels       map[string]string `json:"labels,omitempty"`
	Shutdown     bool              `json:"shutdown,omitempty"`
	AgentVersion string            `json:"agent_version"`
}

type AgentResponse struct {
	Node       types.Node                    `json:"node"`
	Status     types.NodeStatus              `json:"status"`
	Credential *auth.AgentCredential         `json:"credential,omitempty"`
	Directives []controlplane.AgentDirective `json:"directives,omitempty"`
}

type AgentTask = controlplane.AgentTask

type AgentTasksResponse struct {
	Tasks []AgentTask `json:"tasks"`
}

type AgentTaskStatusRequest struct {
	NodeID        types.NodeID     `json:"node_id"`
	Status        types.TaskStatus `json:"status"`
	ContainerID   string           `json:"container_id,omitempty"`
	FailureReason string           `json:"failure_reason,omitempty"`
	ExitCode      *int             `json:"exit_code,omitempty"`
}

type NodeResponse struct {
	Node types.Node `json:"node"`
}

type NodeDrainStatusResponse struct {
	DrainStatus controlplane.NodeDrainStatus `json:"drain_status"`
}

type CreateSecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SecretResponse struct {
	Secret types.SecretMetadata `json:"secret"`
}

type ListSecretsResponse struct {
	Secrets []types.SecretMetadata `json:"secrets"`
}

type CreateRegistryCredentialRequest struct {
	ID       string `json:"id"`
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegistryCredentialResponse struct {
	Credential types.RegistryCredentialMetadata `json:"credential"`
}

type ListRegistryCredentialsResponse struct {
	Credentials []types.RegistryCredentialMetadata `json:"credentials"`
}

type CreateServiceRequest struct {
	Spec types.ServiceSpec `json:"spec"`
}

type ListServicesResponse struct {
	Services []types.Service `json:"services"`
}

type ServiceResponse struct {
	Service types.Service `json:"service"`
}

type ServiceEndpointsResponse = discovery.ServiceEndpoints

type DiscoveryServicesResponse struct {
	Services []discovery.ServiceEndpoints `json:"services"`
}

type TraefikConfigResponse = traefik.Config

type ScaleServiceRequest struct {
	Replicas int `json:"replicas"`
}

type RolloutServiceRequest struct {
	Image          string `json:"image"`
	MaxUnavailable int    `json:"maxUnavailable"`
	MaxSurge       int    `json:"maxSurge"`
}

type DeploymentResponse struct {
	Deployment types.Deployment `json:"deployment"`
}

type ListTasksResponse struct {
	Tasks []types.Task `json:"tasks"`
}

type TaskResponse struct {
	Task types.Task `json:"task"`
}

type ListEventsResponse struct {
	Events []types.Event `json:"events"`
}

type ListAuditLogsResponse struct {
	AuditLogs []audit.Log `json:"audit_logs"`
}

type requestIDKey struct{}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Time: time.Now().UTC()})
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ready", Time: time.Now().UTC()})
}

func (s *Server) apiVersion(w http.ResponseWriter, _ *http.Request) {
	info := versioninfo.Info()
	writeJSON(w, http.StatusOK, VersionResponse{
		APIVersion: info.APIVersion, ServerVersion: info.ServerVersion,
		MinimumAgentVersion: info.MinimumAgentVersion, MaximumTestedAgentVersion: info.MaximumTestedAgentVersion,
		DatabaseSchemaVersion: info.DatabaseSchemaVersion, MinimumSchemaVersion: info.MinimumSchemaVersion,
		MaximumSchemaVersion: info.MaximumSchemaVersion,
	})
}

func (s *Server) createNamespace(w http.ResponseWriter, r *http.Request) {
	var req CreateNamespaceRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	item, err := s.controlPlane.CreateNamespace(r.Context(), req.Name)
	if err != nil {
		s.recordAudit(r, "namespace.create", "namespace", req.Name, audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "namespace.create", "namespace", item.Name, audit.OutcomeSuccess, nil)
	writeJSON(w, http.StatusCreated, NamespaceResponse{Namespace: item})
}

func (s *Server) listNamespaces(w http.ResponseWriter, r *http.Request) {
	items, err := s.controlPlane.ListNamespaces(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListNamespacesResponse{Namespaces: items})
}

func (s *Server) deleteNamespace(w http.ResponseWriter, r *http.Request) {
	name := namespace.Normalize(r.PathValue("name"))
	if err := s.controlPlane.DeleteNamespace(r.Context(), name); err != nil {
		s.recordAudit(r, "namespace.delete", "namespace", name, audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "namespace.delete", "namespace", name, audit.OutcomeSuccess, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getResourceQuota(w http.ResponseWriter, r *http.Request) {
	value, usage, err := s.controlPlane.GetResourceQuota(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ResourceQuotaResponse{Quota: value, Usage: usage})
}

func (s *Server) setResourceQuota(w http.ResponseWriter, r *http.Request) {
	var value types.ResourceQuota
	if !s.decodeJSON(w, r, &value) {
		return
	}
	updated, usage, err := s.controlPlane.SetResourceQuota(r.Context(), value)
	if err != nil {
		s.recordAudit(r, "quota.set", "namespace", namespace.FromContext(r.Context()), audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "quota.set", "namespace", updated.Namespace, audit.OutcomeSuccess, nil)
	writeJSON(w, http.StatusOK, ResourceQuotaResponse{Quota: updated, Usage: usage})
}

func (s *Server) createGitOpsSource(w http.ResponseWriter, r *http.Request) {
	var request CreateGitOpsSourceRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	interval, err := time.ParseDuration(strings.TrimSpace(request.SyncInterval))
	if err != nil || interval <= 0 {
		s.writeError(w, r, fmt.Errorf("%w: sync_interval must be a positive duration", store.ErrInvalidState))
		return
	}
	source, err := s.controlPlane.CreateGitOpsSource(r.Context(), types.GitOpsSource{
		RepositoryURL: request.RepositoryURL, Branch: request.Branch, Path: request.Path,
		SyncInterval: interval, Prune: request.Prune, DriftPolicy: request.DriftPolicy,
	})
	if err != nil {
		s.recordAudit(r, "gitops.source.create", "gitops_source", "unknown", audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "gitops.source.create", "gitops_source", source.ID, audit.OutcomeSuccess, map[string]string{"repository_url": source.RepositoryURL})
	writeJSON(w, http.StatusCreated, GitOpsSourceResponse{Source: source})
}

func (s *Server) listGitOpsSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.controlPlane.ListGitOpsSources(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListGitOpsSourcesResponse{Sources: sources})
}

func (s *Server) deleteGitOpsSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.controlPlane.DeleteGitOpsSource(r.Context(), id); err != nil {
		s.recordAudit(r, "gitops.source.delete", "gitops_source", id, audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "gitops.source.delete", "gitops_source", id, audit.OutcomeSuccess, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncGitOpsSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if s.gitopsSyncer == nil {
		s.writeError(w, r, apperrors.New(apperrors.CodeUnavailable, "GitOps controller is not configured"))
		return
	}
	source, err := s.gitopsSyncer.Sync(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, GitOpsSourceResponse{Source: source})
}

func (s *Server) agentRegister(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAgentRegistration(w, r) {
		return
	}
	var req AgentRegisterRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.NodeName) == "" {
		s.writeError(w, r, fmt.Errorf("%w: node_name is required", store.ErrInvalidState))
		return
	}
	if strings.TrimSpace(req.AdvertiseAddress) == "" {
		s.writeError(w, r, fmt.Errorf("%w: advertise_address is required", store.ErrInvalidState))
		return
	}
	command, err := s.controlPlane.RegisterNode(r.Context(), controlplane.NodeRegistration{
		Name:             req.NodeName,
		AdvertiseAddress: req.AdvertiseAddress,
		Labels:           req.Labels,
		Capacity:         req.Capacity,
		Allocatable:      req.Allocatable,
		AgentVersion:     req.AgentVersion,
	})
	if err != nil {
		s.recordAuditAs(r, audit.ActorAgent, req.NodeName, "agent.register", "node", req.NodeName, audit.OutcomeFailure, map[string]string{
			"node_name": req.NodeName,
		})
		s.writeError(w, r, err)
		return
	}
	credential, err := s.issueAgentCredential(r.Context(), command.Node.ID)
	if err != nil {
		s.recordAuditAs(r, audit.ActorAgent, string(command.Node.ID), "agent.token.rotate", "node", string(command.Node.ID), audit.OutcomeFailure, nil)
		s.recordAuditAs(r, audit.ActorAgent, string(command.Node.ID), "agent.register", "node", string(command.Node.ID), audit.OutcomeFailure, map[string]string{
			"node_name": req.NodeName,
		})
		s.writeError(w, r, err)
		return
	}
	s.recordAuditAs(r, audit.ActorAgent, string(command.Node.ID), "agent.register", "node", string(command.Node.ID), audit.OutcomeSuccess, map[string]string{
		"node_name": req.NodeName,
	})
	s.recordAuditAs(r, audit.ActorAgent, string(command.Node.ID), "agent.token.rotate", "node", string(command.Node.ID), audit.OutcomeSuccess, nil)
	writeJSON(w, http.StatusCreated, AgentResponse{Node: command.Node, Status: command.Node.Status, Credential: &credential, Directives: command.Directives})
}

func (s *Server) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req AgentHeartbeatRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.NodeID == "" {
		s.writeError(w, r, fmt.Errorf("%w: node_id is required", store.ErrInvalidState))
		return
	}
	if !s.authorizeAgentCredential(w, r, req.NodeID) {
		return
	}
	command, err := s.controlPlane.HeartbeatNode(r.Context(), controlplane.NodeHeartbeat{
		NodeID:       req.NodeID,
		Capacity:     req.Capacity,
		Allocatable:  req.Allocatable,
		Labels:       req.Labels,
		Shutdown:     req.Shutdown,
		AgentVersion: req.AgentVersion,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	credential, err := s.issueAgentCredential(r.Context(), command.Node.ID)
	if err != nil {
		s.recordAuditAs(r, audit.ActorAgent, string(command.Node.ID), "agent.token.rotate", "node", string(command.Node.ID), audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAuditAs(r, audit.ActorAgent, string(command.Node.ID), "agent.token.rotate", "node", string(command.Node.ID), audit.OutcomeSuccess, nil)
	writeJSON(w, http.StatusOK, AgentResponse{Node: command.Node, Status: command.Node.Status, Credential: &credential, Directives: command.Directives})
}

func (s *Server) agentListTasks(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
	if !validUUID(nodeID) {
		s.writeError(w, r, fmt.Errorf("%w: node_id must be a UUID", store.ErrInvalidState))
		return
	}
	if !s.authorizeAgentCredential(w, r, types.NodeID(nodeID)) {
		return
	}
	tasks, err := s.controlPlane.ListAssignedTasks(r.Context(), types.NodeID(nodeID))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AgentTasksResponse{Tasks: tasks})
}

func (s *Server) agentTaskStatus(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("task_id"))
	if !validUUID(taskID) {
		s.writeError(w, r, fmt.Errorf("%w: task_id must be a UUID", store.ErrInvalidState))
		return
	}
	var req AgentTaskStatusRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.NodeID == "" {
		s.writeError(w, r, fmt.Errorf("%w: node_id is required", store.ErrInvalidState))
		return
	}
	if !s.authorizeAgentCredential(w, r, req.NodeID) {
		return
	}
	if !types.ValidAgentTaskStatus(req.Status) {
		s.writeError(w, r, fmt.Errorf("%w: task status is invalid", store.ErrInvalidState))
		return
	}
	task, err := s.controlPlane.ReportTaskStatus(r.Context(), controlplane.TaskStatusReport{
		TaskID:        types.TaskID(taskID),
		NodeID:        req.NodeID,
		Status:        req.Status,
		ContainerID:   req.ContainerID,
		FailureReason: req.FailureReason,
		ExitCode:      req.ExitCode,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if req.Status == types.TaskFailed && s.controlMetrics != nil {
		s.controlMetrics.IncTasksFailed()
	}
	writeJSON(w, http.StatusOK, TaskResponse{Task: task})
}

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.controlPlane.ListNodes(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListNodesResponse{Nodes: nodes})
}

func (s *Server) getNode(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathNodeID(w, r)
	if !ok {
		return
	}
	node, err := s.controlPlane.GetNode(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, NodeResponse{Node: node})
}

func (s *Server) drainNode(w http.ResponseWriter, r *http.Request) {
	r = withForceRequest(r)
	id, ok := s.pathNodeID(w, r)
	if !ok {
		return
	}
	node, err := s.controlPlane.DrainNode(r.Context(), id)
	if err != nil {
		s.recordAudit(r, "node.drain", "node", string(id), audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "node.drain", "node", string(id), audit.OutcomeSuccess, nil)
	writeJSON(w, http.StatusOK, NodeResponse{Node: node})
}

func (s *Server) uncordonNode(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathNodeID(w, r)
	if !ok {
		return
	}
	node, err := s.controlPlane.UncordonNode(r.Context(), id)
	if err != nil {
		s.recordAudit(r, "node.uncordon", "node", string(id), audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "node.uncordon", "node", string(id), audit.OutcomeSuccess, nil)
	writeJSON(w, http.StatusOK, NodeResponse{Node: node})
}

func (s *Server) getNodeDrainStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathNodeID(w, r)
	if !ok {
		return
	}
	status, err := s.controlPlane.GetNodeDrainStatus(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, NodeDrainStatusResponse{DrainStatus: status})
}

func (s *Server) createSecret(w http.ResponseWriter, r *http.Request) {
	var req CreateSecretRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		s.writeError(w, r, fmt.Errorf("%w: secret name is required", store.ErrInvalidState))
		return
	}
	secret, err := s.controlPlane.CreateSecret(r.Context(), req.Name, req.Value)
	if err != nil {
		s.recordAudit(r, "secret.create", "secret", req.Name, audit.OutcomeFailure, map[string]string{"name": req.Name})
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "secret.create", "secret", secret.Name, audit.OutcomeSuccess, map[string]string{"name": secret.Name})
	writeJSON(w, http.StatusCreated, SecretResponse{Secret: secret})
}

func (s *Server) listSecrets(w http.ResponseWriter, r *http.Request) {
	secrets, err := s.controlPlane.ListSecrets(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListSecretsResponse{Secrets: secrets})
}

func (s *Server) getSecret(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.writeError(w, r, fmt.Errorf("%w: secret name is required", store.ErrInvalidState))
		return
	}
	secret, err := s.controlPlane.GetSecret(r.Context(), name)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, SecretResponse{Secret: secret})
}

func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.writeError(w, r, fmt.Errorf("%w: secret name is required", store.ErrInvalidState))
		return
	}
	if err := s.controlPlane.DeleteSecret(r.Context(), name); err != nil {
		s.recordAudit(r, "secret.delete", "secret", name, audit.OutcomeFailure, map[string]string{"name": name})
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "secret.delete", "secret", name, audit.OutcomeSuccess, map[string]string{"name": name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createRegistryCredential(w http.ResponseWriter, r *http.Request) {
	var req CreateRegistryCredentialRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	credential, err := s.controlPlane.CreateRegistryCredential(r.Context(), controlplane.RegistryCredentialSpec{
		ID:       req.ID,
		Registry: req.Registry,
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		s.recordAudit(r, "registry_credential.create", "registry_credential", req.ID, audit.OutcomeFailure, map[string]string{
			"registry": req.Registry,
			"username": req.Username,
		})
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "registry_credential.create", "registry_credential", credential.ID, audit.OutcomeSuccess, map[string]string{
		"registry": credential.Registry,
		"username": credential.Username,
	})
	writeJSON(w, http.StatusCreated, RegistryCredentialResponse{Credential: credential})
}

func (s *Server) listRegistryCredentials(w http.ResponseWriter, r *http.Request) {
	credentials, err := s.controlPlane.ListRegistryCredentials(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListRegistryCredentialsResponse{Credentials: credentials})
}

func (s *Server) deleteRegistryCredential(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		s.writeError(w, r, fmt.Errorf("%w: registry credential id is required", store.ErrInvalidState))
		return
	}
	if err := s.controlPlane.DeleteRegistryCredential(r.Context(), id); err != nil {
		s.recordAudit(r, "registry_credential.delete", "registry_credential", id, audit.OutcomeFailure, map[string]string{"id": id})
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "registry_credential.delete", "registry_credential", id, audit.OutcomeSuccess, map[string]string{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var req CreateServiceRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := req.Spec.Validate(); err != nil {
		s.writeError(w, r, fmt.Errorf("%w: %v", store.ErrInvalidState, err))
		return
	}
	service, err := s.controlPlane.CreateService(r.Context(), req.Spec)
	if err != nil {
		s.recordAudit(r, "service.create", "service", req.Spec.Name, audit.OutcomeFailure, map[string]string{
			"image": req.Spec.Image,
			"name":  req.Spec.Name,
		})
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "service.create", "service", string(service.ID), audit.OutcomeSuccess, map[string]string{
		"image": service.Spec.Image,
		"name":  service.Spec.Name,
	})
	writeJSON(w, http.StatusCreated, ServiceResponse{Service: service})
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	services, err := s.controlPlane.ListServices(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListServicesResponse{Services: services})
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathServiceID(w, r)
	if !ok {
		return
	}
	service, err := s.controlPlane.GetService(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ServiceResponse{Service: service})
}

func (s *Server) getServiceEndpoints(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathServiceID(w, r)
	if !ok {
		return
	}
	includeUnhealthy := includeUnhealthy(r)
	service, err := s.controlPlane.GetService(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	tasks, err := s.controlPlane.ListTasks(r.Context(), controlplane.TaskFilter{ServiceID: id})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	nodes, err := s.controlPlane.ListNodes(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, discovery.BuildServiceEndpoints(service, tasks, nodes, includeUnhealthy))
}

func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathServiceID(w, r)
	if !ok {
		return
	}
	if err := s.controlPlane.DeleteService(r.Context(), id); err != nil {
		s.recordAudit(r, "service.delete", "service", string(id), audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "service.delete", "service", string(id), audit.OutcomeSuccess, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) scaleService(w http.ResponseWriter, r *http.Request) {
	r = withForceRequest(r)
	id, ok := s.pathServiceID(w, r)
	if !ok {
		return
	}
	var req ScaleServiceRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.Replicas < 0 {
		s.writeError(w, r, fmt.Errorf("%w: replicas cannot be negative", store.ErrInvalidState))
		return
	}
	service, err := s.controlPlane.ScaleService(r.Context(), id, req.Replicas)
	if err != nil {
		s.recordAudit(r, "service.scale", "service", string(id), audit.OutcomeFailure, map[string]string{
			"replicas": strconv.Itoa(req.Replicas),
		})
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "service.scale", "service", string(id), audit.OutcomeSuccess, map[string]string{
		"replicas": strconv.Itoa(service.Spec.Replicas),
	})
	writeJSON(w, http.StatusOK, ServiceResponse{Service: service})
}

func (s *Server) rolloutService(w http.ResponseWriter, r *http.Request) {
	r = withForceRequest(r)
	id, ok := s.pathServiceID(w, r)
	if !ok {
		return
	}
	var req RolloutServiceRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	req.Image = strings.TrimSpace(req.Image)
	if req.Image == "" {
		s.writeError(w, r, fmt.Errorf("%w: image is required", store.ErrInvalidState))
		return
	}
	if req.MaxUnavailable < 0 {
		s.writeError(w, r, fmt.Errorf("%w: maxUnavailable cannot be negative", store.ErrInvalidState))
		return
	}
	if req.MaxSurge < 0 {
		s.writeError(w, r, fmt.Errorf("%w: maxSurge cannot be negative", store.ErrInvalidState))
		return
	}
	if req.MaxUnavailable == 0 && req.MaxSurge == 0 {
		req.MaxUnavailable = 1
		req.MaxSurge = 1
	}
	deployment, err := s.controlPlane.RolloutService(r.Context(), id, controlplane.RolloutSpec{
		Image:          req.Image,
		MaxUnavailable: req.MaxUnavailable,
		MaxSurge:       req.MaxSurge,
	})
	if err != nil {
		s.recordAudit(r, "service.rollout", "service", string(id), audit.OutcomeFailure, map[string]string{
			"image":           req.Image,
			"max_surge":       strconv.Itoa(req.MaxSurge),
			"max_unavailable": strconv.Itoa(req.MaxUnavailable),
		})
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "service.rollout", "service", string(id), audit.OutcomeSuccess, map[string]string{
		"deployment_id":   string(deployment.ID),
		"image":           req.Image,
		"max_surge":       strconv.Itoa(req.MaxSurge),
		"max_unavailable": strconv.Itoa(req.MaxUnavailable),
		"target_version":  strconv.FormatInt(deployment.ToVersion, 10),
	})
	if s.controlMetrics != nil {
		s.controlMetrics.IncRollouts()
	}
	writeJSON(w, http.StatusAccepted, DeploymentResponse{Deployment: deployment})
}

func (s *Server) getServiceRollout(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathServiceID(w, r)
	if !ok {
		return
	}
	deployment, err := s.controlPlane.GetServiceRollout(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, DeploymentResponse{Deployment: deployment})
}

func (s *Server) getRollout(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if !validUUID(id) {
		s.writeError(w, r, fmt.Errorf("%w: rollout id must be a UUID", store.ErrInvalidState))
		return
	}
	deployment, err := s.controlPlane.GetDeployment(r.Context(), types.DeploymentID(id))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, DeploymentResponse{Deployment: deployment})
}

func (s *Server) rollbackService(w http.ResponseWriter, r *http.Request) {
	r = withForceRequest(r)
	id, ok := s.pathServiceID(w, r)
	if !ok {
		return
	}
	deployment, err := s.controlPlane.RollbackService(r.Context(), id)
	if err != nil {
		s.recordAudit(r, "service.rollback", "service", string(id), audit.OutcomeFailure, nil)
		s.writeError(w, r, err)
		return
	}
	s.recordAudit(r, "service.rollback", "service", string(id), audit.OutcomeSuccess, map[string]string{
		"deployment_id":  string(deployment.ID),
		"target_version": strconv.FormatInt(deployment.ToVersion, 10),
	})
	if s.controlMetrics != nil {
		s.controlMetrics.IncRollouts()
	}
	writeJSON(w, http.StatusAccepted, DeploymentResponse{Deployment: deployment})
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	filter, ok := s.taskFilter(w, r)
	if !ok {
		return
	}
	tasks, err := s.controlPlane.ListTasks(r.Context(), filter)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListTasksResponse{Tasks: tasks})
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathTaskID(w, r)
	if !ok {
		return
	}
	task, err := s.controlPlane.GetTask(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, TaskResponse{Task: task})
}

func (s *Server) discoveryServices(w http.ResponseWriter, r *http.Request) {
	includeUnhealthy := includeUnhealthy(r)
	services, tasks, nodes, err := s.discoverySnapshot(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, DiscoveryServicesResponse{
		Services: discovery.BuildAllServiceEndpoints(services, tasks, nodes, includeUnhealthy),
	})
}

func (s *Server) discoveryServiceByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.writeError(w, r, fmt.Errorf("%w: service name is required", store.ErrInvalidState))
		return
	}
	includeUnhealthy := includeUnhealthy(r)
	services, tasks, nodes, err := s.discoverySnapshot(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	for _, service := range services {
		if service.Spec.Name == name {
			writeJSON(w, http.StatusOK, discovery.BuildServiceEndpoints(service, tasks, nodes, includeUnhealthy))
			return
		}
	}
	s.writeError(w, r, fmt.Errorf("%w: service %q not found", store.ErrNotFound, name))
}

func (s *Server) traefikConfig(w http.ResponseWriter, r *http.Request) {
	services, tasks, nodes, err := s.discoverySnapshot(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	endpointSets := discovery.BuildAllServiceEndpoints(services, tasks, nodes, false)
	writeJSON(w, http.StatusOK, traefik.BuildConfig(services, endpointSets))
}

func (s *Server) discoverySnapshot(ctx context.Context) ([]types.Service, []types.Task, []types.Node, error) {
	services, err := s.controlPlane.ListServices(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	tasks, err := s.controlPlane.ListTasks(ctx, controlplane.TaskFilter{})
	if err != nil {
		return nil, nil, nil, err
	}
	nodes, err := s.controlPlane.ListNodes(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	return services, tasks, nodes, nil
}

func includeUnhealthy(r *http.Request) bool {
	value := strings.TrimSpace(r.URL.Query().Get("include_unhealthy"))
	return value == "true" || value == "1"
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	filter, ok := s.eventFilter(w, r)
	if !ok {
		return
	}
	events, err := s.controlPlane.ListEvents(r.Context(), filter)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListEventsResponse{Events: events})
}

func (s *Server) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	if s.auditStore == nil {
		s.writeError(w, r, apperrors.New(apperrors.CodeUnavailable, "audit store is not configured"))
		return
	}
	filter, ok := s.auditFilter(w, r)
	if !ok {
		return
	}
	logs, err := s.auditStore.ListAuditLogs(r.Context(), filter)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ListAuditLogsResponse{AuditLogs: logs})
}

func (s *Server) streamLogs(w http.ResponseWriter, r *http.Request) {
	task, err := s.logTask(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if task.NodeID == "" {
		s.writeError(w, r, fmt.Errorf("%w: task is not assigned to a node", store.ErrInvalidState))
		return
	}
	if task.ContainerID == "" {
		s.writeError(w, r, fmt.Errorf("%w: task has no container yet", store.ErrInvalidState))
		return
	}
	node, err := s.controlPlane.GetNode(r.Context(), task.NodeID)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if node.Status == types.NodeOffline || node.Status == types.NodeUnknown {
		s.writeError(w, r, fmt.Errorf("%w: node %s is %s", store.ErrInvalidState, node.ID, node.Status))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	if err := s.logStreamer.StreamLogs(r.Context(), LogStreamRequest{
		AgentURL: node.AdvertiseAddress,
		TaskID:   string(task.ID),
		Follow:   r.URL.Query().Get("follow") == "true",
		Tail:     strings.TrimSpace(r.URL.Query().Get("tail")),
		Token:    s.bootstrapToken,
	}, w); err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Warn("log stream failed", "task_id", task.ID, "error", err)
	}
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return requestIDMiddleware(apiVersionMiddleware(s.accessLogMiddleware(s.recoveryMiddleware(s.timeoutMiddleware(s.userAuthMiddleware(next))))))
}

func (s *Server) userAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		namespaceName := selectedNamespace(r)
		if err := namespace.Validate(namespaceName); err != nil {
			s.writeError(w, r, fmt.Errorf("%w: %v", store.ErrInvalidState, err))
			return
		}
		r = r.WithContext(namespace.WithContext(r.Context(), namespaceName))
		required, ok := requiredRole(r)
		if !ok || s.jwtSecret == "" {
			next.ServeHTTP(w, r)
			return
		}
		token, err := auth.ParseBearer(r.Header.Get("Authorization"))
		if err != nil {
			s.writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}
		principal, err := auth.ValidateJWT(token, s.jwtSecret, s.now())
		if err != nil {
			s.writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "invalid bearer token")
			return
		}
		if len(s.staticUsers) > 0 {
			role, ok := s.staticUsers[principal.Subject]
			if !ok {
				s.writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "unknown user")
				return
			}
			principal.Role = role
			principal.NamespaceRoles = nil
		}
		role, hasNamespaceRole := principal.RoleForNamespace(namespaceName)
		if strings.HasPrefix(r.URL.Path, "/v1/namespaces") && !auth.ValidRole(principal.Role) {
			hasNamespaceRole = false
		}
		if !hasNamespaceRole || !auth.HasRole(role, required) {
			s.writeAuthError(w, r, http.StatusForbidden, "forbidden", "insufficient role")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

func selectedNamespace(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Orch-Namespace")); value != "" {
		return namespace.Normalize(value)
	}
	return namespace.Normalize(r.URL.Query().Get("namespace"))
}

func requiredRole(r *http.Request) (auth.Role, bool) {
	path := r.URL.Path
	if path == "/healthz" || path == "/readyz" || path == "/metrics" || strings.HasPrefix(path, "/v1/agent/") {
		return "", false
	}
	if r.Method == http.MethodGet {
		switch {
		case path == "/v1/version":
			return auth.RoleViewer, true
		case path == "/v1/namespaces":
			return auth.RoleAdmin, true
		case path == "/v1/quota":
			return auth.RoleViewer, true
		case path == "/v1/gitops/sources" || strings.HasPrefix(path, "/v1/gitops/sources/"):
			return auth.RoleViewer, true
		case path == "/v1/nodes" || strings.HasPrefix(path, "/v1/nodes/"):
			return auth.RoleViewer, true
		case path == "/v1/secrets" || strings.HasPrefix(path, "/v1/secrets/"):
			return auth.RoleViewer, true
		case path == "/v1/registry-credentials" || strings.HasPrefix(path, "/v1/registry-credentials/"):
			return auth.RoleViewer, true
		case path == "/v1/services" || strings.HasPrefix(path, "/v1/services/"):
			return auth.RoleViewer, true
		case path == "/v1/tasks" || strings.HasPrefix(path, "/v1/tasks/"):
			return auth.RoleViewer, true
		case strings.HasPrefix(path, "/v1/discovery/"):
			return auth.RoleViewer, true
		case path == "/v1/integrations/traefik/config":
			return auth.RoleViewer, true
		case path == "/v1/events":
			return auth.RoleViewer, true
		case path == "/v1/audit":
			return auth.RoleAdmin, true
		case path == "/v1/logs":
			return auth.RoleViewer, true
		case strings.HasPrefix(path, "/v1/rollouts/"):
			return auth.RoleViewer, true
		}
	}
	if r.Method == http.MethodPost {
		switch {
		case path == "/v1/namespaces":
			return auth.RoleAdmin, true
		case path == "/v1/gitops/sources" || strings.HasSuffix(path, "/sync"):
			return auth.RoleOperator, true
		case strings.HasSuffix(path, "/drain") || strings.HasSuffix(path, "/uncordon"):
			return auth.RoleAdmin, true
		case path == "/v1/secrets":
			return auth.RoleOperator, true
		case path == "/v1/registry-credentials":
			return auth.RoleOperator, true
		case path == "/v1/services" || strings.Contains(path, "/scale") || strings.Contains(path, "/rollout") || strings.Contains(path, "/rollback"):
			return auth.RoleOperator, true
		}
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(path, "/v1/namespaces/") {
		return auth.RoleAdmin, true
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(path, "/v1/gitops/sources/") {
		return auth.RoleOperator, true
	}
	if r.Method == http.MethodPut && path == "/v1/quota" {
		return auth.RoleAdmin, true
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(path, "/v1/secrets/") {
		return auth.RoleOperator, true
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(path, "/v1/registry-credentials/") {
		return auth.RoleOperator, true
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(path, "/v1/services/") {
		return auth.RoleOperator, true
	}
	return auth.RoleAdmin, true
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) timeoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/logs" && r.URL.Query().Get("follow") == "true" {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("panic recovered",
					"request_id", requestID(r.Context()),
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				s.writeError(w, r, errors.New("internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now().UTC()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if s.metrics != nil {
			if route, ok := metricRoute(r.Method, r.URL.Path); ok {
				s.metrics.ObserveAPIRequest(r.Method, route, rec.status, time.Since(started))
			}
		}
		s.logger.Info("http request",
			"request_id", requestID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func metricRoute(_ string, path string) (string, bool) {
	if path == "/metrics" {
		return "", false
	}
	switch {
	case path == "/healthz":
		return "/healthz", true
	case path == "/readyz":
		return "/readyz", true
	case path == "/v1/version":
		return "/v1/version", true
	case path == "/v1/namespaces":
		return "/v1/namespaces", true
	case strings.HasPrefix(path, "/v1/namespaces/"):
		return "/v1/namespaces/{name}", true
	case path == "/v1/quota":
		return "/v1/quota", true
	case path == "/v1/gitops/sources":
		return "/v1/gitops/sources", true
	case strings.HasPrefix(path, "/v1/gitops/sources/") && strings.HasSuffix(path, "/sync"):
		return "/v1/gitops/sources/{id}/sync", true
	case strings.HasPrefix(path, "/v1/gitops/sources/"):
		return "/v1/gitops/sources/{id}", true
	case path == "/v1/agent/register":
		return "/v1/agent/register", true
	case path == "/v1/agent/heartbeat":
		return "/v1/agent/heartbeat", true
	case path == "/v1/agent/tasks":
		return "/v1/agent/tasks", true
	case strings.HasPrefix(path, "/v1/agent/tasks/") && strings.HasSuffix(path, "/status"):
		return "/v1/agent/tasks/{task_id}/status", true
	case path == "/v1/nodes":
		return "/v1/nodes", true
	case strings.HasPrefix(path, "/v1/nodes/") && strings.HasSuffix(path, "/drain"):
		return "/v1/nodes/{id}/drain", true
	case strings.HasPrefix(path, "/v1/nodes/") && strings.HasSuffix(path, "/uncordon"):
		return "/v1/nodes/{id}/uncordon", true
	case strings.HasPrefix(path, "/v1/nodes/") && strings.HasSuffix(path, "/drain-status"):
		return "/v1/nodes/{id}/drain-status", true
	case strings.HasPrefix(path, "/v1/nodes/"):
		return "/v1/nodes/{id}", true
	case path == "/v1/secrets":
		return "/v1/secrets", true
	case strings.HasPrefix(path, "/v1/secrets/"):
		return "/v1/secrets/{name}", true
	case path == "/v1/registry-credentials":
		return "/v1/registry-credentials", true
	case strings.HasPrefix(path, "/v1/registry-credentials/"):
		return "/v1/registry-credentials/{id}", true
	case path == "/v1/services":
		return "/v1/services", true
	case strings.HasPrefix(path, "/v1/services/") && strings.HasSuffix(path, "/scale"):
		return "/v1/services/{id}/scale", true
	case strings.HasPrefix(path, "/v1/services/") && strings.HasSuffix(path, "/rollout"):
		return "/v1/services/{id}/rollout", true
	case strings.HasPrefix(path, "/v1/services/") && strings.HasSuffix(path, "/rollback"):
		return "/v1/services/{id}/rollback", true
	case strings.HasPrefix(path, "/v1/services/"):
		return "/v1/services/{id}", true
	case path == "/v1/tasks":
		return "/v1/tasks", true
	case strings.HasPrefix(path, "/v1/tasks/"):
		return "/v1/tasks/{id}", true
	case path == "/v1/discovery/services":
		return "/v1/discovery/services", true
	case strings.HasPrefix(path, "/v1/discovery/services/"):
		return "/v1/discovery/services/{name}", true
	case path == "/v1/integrations/traefik/config":
		return "/v1/integrations/traefik/config", true
	case path == "/v1/events":
		return "/v1/events", true
	case path == "/v1/audit":
		return "/v1/audit", true
	case path == "/v1/logs":
		return "/v1/logs", true
	case strings.HasPrefix(path, "/v1/rollouts/"):
		return "/v1/rollouts/{id}", true
	default:
		return "unknown", true
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		s.writeError(w, r, fmt.Errorf("%w: invalid JSON body: %v", store.ErrInvalidState, err))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		s.writeError(w, r, fmt.Errorf("%w: request body must contain a single JSON object", store.ErrInvalidState))
		return false
	}
	return true
}

func (s *Server) authorizeAgentRegistration(w http.ResponseWriter, r *http.Request) bool {
	if s.bootstrapToken == "" {
		return true
	}
	token := strings.TrimSpace(r.Header.Get("X-Orch-Bootstrap-Token"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if token != s.bootstrapToken {
		s.writeAuthError(w, r, http.StatusUnauthorized, string(apperrors.CodeUnauthorized), "invalid agent registration token")
		return false
	}
	return true
}

func (s *Server) authorizeAgentCredential(w http.ResponseWriter, r *http.Request, nodeID types.NodeID) bool {
	token, err := auth.ParseBearer(r.Header.Get("Authorization"))
	if err != nil {
		s.writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "missing or invalid agent credential")
		return false
	}
	node, err := s.controlPlane.GetNode(r.Context(), nodeID)
	if err != nil {
		s.writeError(w, r, err)
		return false
	}
	record := auth.AgentCredentialRecord{
		Hash:      node.AgentTokenHash,
		ExpiresAt: node.AgentTokenExpiry,
		Revoked:   node.AgentRevoked,
	}
	if err := s.agentIssuer.Validate(r.Context(), token, record, s.now()); err != nil {
		s.writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "invalid agent credential")
		return false
	}
	return true
}

func (s *Server) issueAgentCredential(ctx context.Context, nodeID types.NodeID) (auth.AgentCredential, error) {
	credential, record, err := s.agentIssuer.Issue(ctx, auth.AgentIdentity{NodeID: string(nodeID)})
	if err != nil {
		return auth.AgentCredential{}, err
	}
	if _, err := s.controlPlane.SetAgentCredential(ctx, nodeID, record.Hash, record.ExpiresAt); err != nil {
		return auth.AgentCredential{}, err
	}
	return credential, nil
}

func (s *Server) writeAuthError(w http.ResponseWriter, r *http.Request, status int, code string, message string) {
	s.logRequestError(r, status, apperrors.Code(code), errors.New(message))
	writeJSON(w, status, ErrorResponse{
		Error: RequestError{
			Code:      code,
			Message:   message,
			RequestID: requestID(r.Context()),
		},
	})
}

func (s *Server) pathNodeID(w http.ResponseWriter, r *http.Request) (types.NodeID, bool) {
	id := r.PathValue("id")
	if !validUUID(id) {
		s.writeError(w, r, fmt.Errorf("%w: node id must be a UUID", store.ErrInvalidState))
		return "", false
	}
	return types.NodeID(id), true
}

func (s *Server) pathServiceID(w http.ResponseWriter, r *http.Request) (types.ServiceID, bool) {
	id := r.PathValue("id")
	if !validUUID(id) {
		s.writeError(w, r, fmt.Errorf("%w: service id must be a UUID", store.ErrInvalidState))
		return "", false
	}
	return types.ServiceID(id), true
}

func (s *Server) pathTaskID(w http.ResponseWriter, r *http.Request) (types.TaskID, bool) {
	id := r.PathValue("id")
	if !validUUID(id) {
		s.writeError(w, r, fmt.Errorf("%w: task id must be a UUID", store.ErrInvalidState))
		return "", false
	}
	return types.TaskID(id), true
}

func (s *Server) taskFilter(w http.ResponseWriter, r *http.Request) (controlplane.TaskFilter, bool) {
	query := r.URL.Query()
	var filter controlplane.TaskFilter
	if serviceID := strings.TrimSpace(query.Get("service_id")); serviceID != "" {
		if !validUUID(serviceID) {
			s.writeError(w, r, fmt.Errorf("%w: service_id must be a UUID", store.ErrInvalidState))
			return controlplane.TaskFilter{}, false
		}
		filter.ServiceID = types.ServiceID(serviceID)
	}
	if nodeID := strings.TrimSpace(query.Get("node_id")); nodeID != "" {
		if !validUUID(nodeID) {
			s.writeError(w, r, fmt.Errorf("%w: node_id must be a UUID", store.ErrInvalidState))
			return controlplane.TaskFilter{}, false
		}
		filter.NodeID = types.NodeID(nodeID)
	}
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		if !types.ValidTaskStatus(types.TaskStatus(status)) {
			s.writeError(w, r, fmt.Errorf("%w: status is invalid", store.ErrInvalidState))
			return controlplane.TaskFilter{}, false
		}
		filter.Status = types.TaskStatus(status)
	}
	return filter, true
}

func (s *Server) eventFilter(w http.ResponseWriter, r *http.Request) (events.Filter, bool) {
	query := r.URL.Query()
	limit := 100
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 500 {
			s.writeError(w, r, fmt.Errorf("%w: limit must be between 1 and 500", store.ErrInvalidState))
			return events.Filter{}, false
		}
		limit = parsed
	}
	var filter events.Filter
	filter.Limit = limit
	if serviceID := strings.TrimSpace(query.Get("service_id")); serviceID != "" {
		if !validUUID(serviceID) {
			s.writeError(w, r, fmt.Errorf("%w: service_id must be a UUID", store.ErrInvalidState))
			return events.Filter{}, false
		}
		filter.ServiceID = types.ServiceID(serviceID)
	}
	if taskID := strings.TrimSpace(query.Get("task_id")); taskID != "" {
		if !validUUID(taskID) {
			s.writeError(w, r, fmt.Errorf("%w: task_id must be a UUID", store.ErrInvalidState))
			return events.Filter{}, false
		}
		filter.TaskID = types.TaskID(taskID)
	}
	if nodeID := strings.TrimSpace(query.Get("node_id")); nodeID != "" {
		if !validUUID(nodeID) {
			s.writeError(w, r, fmt.Errorf("%w: node_id must be a UUID", store.ErrInvalidState))
			return events.Filter{}, false
		}
		filter.NodeID = types.NodeID(nodeID)
	}
	filter.Type = strings.TrimSpace(query.Get("type"))
	if severity := strings.TrimSpace(query.Get("severity")); severity != "" {
		if !validEventSeverity(types.EventSeverity(severity)) {
			s.writeError(w, r, fmt.Errorf("%w: severity is invalid", store.ErrInvalidState))
			return events.Filter{}, false
		}
		filter.Severity = types.EventSeverity(severity)
	}
	if since := strings.TrimSpace(query.Get("since")); since != "" {
		parsed, err := time.Parse(time.RFC3339, since)
		if err != nil {
			s.writeError(w, r, fmt.Errorf("%w: since must be RFC3339", store.ErrInvalidState))
			return events.Filter{}, false
		}
		filter.Since = parsed.UTC()
	}
	return filter, true
}

func (s *Server) auditFilter(w http.ResponseWriter, r *http.Request) (audit.Filter, bool) {
	query := r.URL.Query()
	limit := 100
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 500 {
			s.writeError(w, r, fmt.Errorf("%w: limit must be between 1 and 500", store.ErrInvalidState))
			return audit.Filter{}, false
		}
		limit = parsed
	}
	filter := audit.Filter{
		ActorID:    strings.TrimSpace(query.Get("actor_id")),
		Action:     strings.TrimSpace(query.Get("action")),
		TargetType: strings.TrimSpace(query.Get("target_type")),
		TargetID:   strings.TrimSpace(query.Get("target_id")),
		Limit:      limit,
	}
	if actorType := strings.TrimSpace(query.Get("actor_type")); actorType != "" {
		if !validAuditActorType(audit.ActorType(actorType)) {
			s.writeError(w, r, fmt.Errorf("%w: actor_type is invalid", store.ErrInvalidState))
			return audit.Filter{}, false
		}
		filter.ActorType = audit.ActorType(actorType)
	}
	if outcome := strings.TrimSpace(query.Get("outcome")); outcome != "" {
		if !validAuditOutcome(audit.Outcome(outcome)) {
			s.writeError(w, r, fmt.Errorf("%w: outcome is invalid", store.ErrInvalidState))
			return audit.Filter{}, false
		}
		filter.Outcome = audit.Outcome(outcome)
	}
	if since := strings.TrimSpace(query.Get("since")); since != "" {
		parsed, err := time.Parse(time.RFC3339, since)
		if err != nil {
			s.writeError(w, r, fmt.Errorf("%w: since must be RFC3339", store.ErrInvalidState))
			return audit.Filter{}, false
		}
		filter.Since = parsed.UTC()
	}
	return filter, true
}

func (s *Server) logTask(r *http.Request) (types.Task, error) {
	query := r.URL.Query()
	taskID := strings.TrimSpace(query.Get("task_id"))
	serviceID := strings.TrimSpace(query.Get("service_id"))
	if taskID == "" && serviceID == "" {
		return types.Task{}, fmt.Errorf("%w: service_id or task_id is required", store.ErrInvalidState)
	}
	if taskID != "" {
		if !validUUID(taskID) {
			return types.Task{}, fmt.Errorf("%w: task_id must be a UUID", store.ErrInvalidState)
		}
		return s.controlPlane.GetTask(r.Context(), types.TaskID(taskID))
	}
	if !validUUID(serviceID) {
		return types.Task{}, fmt.Errorf("%w: service_id must be a UUID", store.ErrInvalidState)
	}
	tasks, err := s.controlPlane.ListTasks(r.Context(), controlplane.TaskFilter{ServiceID: types.ServiceID(serviceID)})
	if err != nil {
		return types.Task{}, err
	}
	for _, task := range tasks {
		if task.ContainerID == "" || task.NodeID == "" {
			continue
		}
		if task.ActualStatus == types.TaskRunning || task.ActualStatus == types.TaskHealthy || task.ActualStatus == types.TaskUnhealthy {
			return task, nil
		}
	}
	return types.Task{}, store.ErrNotFound
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message, details := errorAttributes(err)
	s.logRequestError(r, status, code, err)
	writeJSON(w, status, ErrorResponse{
		Error: RequestError{
			Code:      string(code),
			Message:   message,
			RequestID: requestID(r.Context()),
			Details:   details,
		},
	})
}

func (s *Server) recordAudit(r *http.Request, action string, targetType string, targetID string, outcome audit.Outcome, metadata map[string]string) {
	actorType, actorID := auditActor(r)
	s.recordAuditAs(r, actorType, actorID, action, targetType, targetID, outcome, metadata)
}

func (s *Server) recordAuditAs(r *http.Request, actorType audit.ActorType, actorID string, action string, targetType string, targetID string, outcome audit.Outcome, metadata map[string]string) {
	if s.auditStore == nil {
		return
	}
	log := audit.Log{
		Namespace:  namespace.FromContext(r.Context()),
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		RequestID:  requestID(r.Context()),
		SourceIP:   sourceIP(r),
		Outcome:    outcome,
		Metadata:   audit.RedactMetadata(metadata),
		Timestamp:  s.now(),
	}
	if _, err := s.auditStore.AppendAuditLog(r.Context(), log); err != nil && s.logger != nil {
		s.logger.Warn("audit log append failed",
			"request_id", requestID(r.Context()),
			"action", action,
			"target_type", targetType,
			"target_id", targetID,
			"outcome", string(outcome),
			"error", err,
		)
	}
}

func auditActor(r *http.Request) (audit.ActorType, string) {
	if principal, ok := auth.PrincipalFromContext(r.Context()); ok && strings.TrimSpace(principal.Subject) != "" {
		return audit.ActorUser, principal.Subject
	}
	if strings.HasPrefix(r.URL.Path, "/v1/agent/") {
		if nodeID := strings.TrimSpace(r.URL.Query().Get("node_id")); nodeID != "" {
			return audit.ActorAgent, nodeID
		}
		return audit.ActorAgent, "unknown"
	}
	return audit.ActorUser, "anonymous"
}

func sourceIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if first := strings.TrimSpace(parts[0]); first != "" {
			return first
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func errorAttributes(err error) (int, apperrors.Code, string, map[string]any) {
	var admissionErr *admission.Error
	if errors.As(err, &admissionErr) {
		return http.StatusBadRequest, apperrors.CodeInvalidArgument, admissionErr.Error(), map[string]any{
			"operation":  admissionErr.Operation,
			"namespace":  admissionErr.Namespace,
			"violations": admissionErr.Violations,
		}
	}
	var quotaErr *quota.Error
	if errors.As(err, &quotaErr) {
		return http.StatusConflict, apperrors.CodeQuotaExceeded, quotaErr.Error(), map[string]any{
			"namespace": quotaErr.Namespace, "resource": quotaErr.Resource, "limit": quotaErr.Limit,
			"used": quotaErr.Used, "requested": quotaErr.Requested,
		}
	}
	code := apperrors.CodeOf(err)
	message := apperrors.MessageOf(err)
	details := apperrors.DetailsOf(err)
	return statusForCode(code), code, message, details
}

func statusForCode(code apperrors.Code) int {
	switch {
	case code == apperrors.CodeNotFound:
		return http.StatusNotFound
	case code == apperrors.CodeInvalidArgument:
		return http.StatusBadRequest
	case code == apperrors.CodeConflict:
		return http.StatusConflict
	case code == apperrors.CodeQuotaExceeded:
		return http.StatusConflict
	case code == apperrors.CodeUnauthorized:
		return http.StatusUnauthorized
	case code == apperrors.CodeForbidden:
		return http.StatusForbidden
	case code == apperrors.CodeFailedPrecondition:
		return http.StatusPreconditionFailed
	case code == apperrors.CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func (s *Server) logRequestError(r *http.Request, status int, code apperrors.Code, err error) {
	if s.logger == nil {
		return
	}
	fields := []any{
		"request_id", requestID(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"error_code", string(code),
		"error", apperrors.MessageOf(err),
	}
	fields = append(fields, requestObjectFields(r)...)
	if status >= http.StatusInternalServerError {
		s.logger.Error("api request failed", fields...)
		return
	}
	s.logger.Warn("api request failed", fields...)
}

func requestObjectFields(r *http.Request) []any {
	fields := make([]any, 0, 8)
	if id := strings.TrimSpace(r.PathValue("id")); id != "" {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/services/"):
			fields = append(fields, "service_id", id)
		case strings.HasPrefix(r.URL.Path, "/v1/nodes/"):
			fields = append(fields, "node_id", id)
		case strings.HasPrefix(r.URL.Path, "/v1/tasks/"):
			fields = append(fields, "task_id", id)
		default:
			fields = append(fields, "object_id", id)
		}
	}
	if taskID := strings.TrimSpace(r.PathValue("task_id")); taskID != "" {
		fields = append(fields, "task_id", taskID)
	}
	query := r.URL.Query()
	for _, item := range []struct {
		name  string
		field string
	}{
		{name: "service_id", field: "service_id"},
		{name: "task_id", field: "task_id"},
		{name: "node_id", field: "node_id"},
	} {
		if value := strings.TrimSpace(query.Get(item.name)); value != "" {
			fields = append(fields, item.field, value)
		}
	}
	return fields
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

func validEventSeverity(severity types.EventSeverity) bool {
	switch severity {
	case types.EventInfo, types.EventWarning, types.EventError:
		return true
	default:
		return false
	}
}

func validAuditActorType(actorType audit.ActorType) bool {
	switch actorType {
	case audit.ActorUser, audit.ActorAgent, audit.ActorSystem:
		return true
	default:
		return false
	}
}

func validAuditOutcome(outcome audit.Outcome) bool {
	switch outcome {
	case audit.OutcomeSuccess, audit.OutcomeFailure:
		return true
	default:
		return false
	}
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
