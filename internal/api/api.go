package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/auth"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/discovery"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/internal/traefik"
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
	now            func() time.Time
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

func NewHandler(logger *slog.Logger, controlPlane controlplane.Service, opts ...Option) http.Handler {
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
	mux.HandleFunc("POST /v1/agent/register", server.agentRegister)
	mux.HandleFunc("POST /v1/agent/heartbeat", server.agentHeartbeat)
	mux.HandleFunc("GET /v1/agent/tasks", server.agentListTasks)
	mux.HandleFunc("POST /v1/agent/tasks/{task_id}/status", server.agentTaskStatus)
	mux.HandleFunc("GET /v1/nodes", server.listNodes)
	mux.HandleFunc("GET /v1/nodes/{id}", server.getNode)
	mux.HandleFunc("POST /v1/nodes/{id}/drain", server.drainNode)
	mux.HandleFunc("POST /v1/nodes/{id}/uncordon", server.uncordonNode)
	mux.HandleFunc("POST /v1/secrets", server.createSecret)
	mux.HandleFunc("GET /v1/secrets", server.listSecrets)
	mux.HandleFunc("GET /v1/secrets/{name...}", server.getSecret)
	mux.HandleFunc("DELETE /v1/secrets/{name...}", server.deleteSecret)
	mux.HandleFunc("POST /v1/registry-credentials", server.createRegistryCredential)
	mux.HandleFunc("GET /v1/registry-credentials", server.listRegistryCredentials)
	mux.HandleFunc("DELETE /v1/registry-credentials/{id}", server.deleteRegistryCredential)
	mux.HandleFunc("POST /v1/services", server.createService)
	mux.HandleFunc("GET /v1/services", server.listServices)
	mux.HandleFunc("GET /v1/services/{id}", server.getService)
	mux.HandleFunc("GET /v1/services/{id}/endpoints", server.getServiceEndpoints)
	mux.HandleFunc("DELETE /v1/services/{id}", server.deleteService)
	mux.HandleFunc("POST /v1/services/{id}/scale", server.scaleService)
	mux.HandleFunc("POST /v1/services/{id}/rollout", server.rolloutService)
	mux.HandleFunc("GET /v1/services/{id}/rollout", server.getServiceRollout)
	mux.HandleFunc("GET /v1/rollouts/{id}", server.getRollout)
	mux.HandleFunc("POST /v1/services/{id}/rollback", server.rollbackService)
	mux.HandleFunc("GET /v1/tasks", server.listTasks)
	mux.HandleFunc("GET /v1/tasks/{id}", server.getTask)
	mux.HandleFunc("GET /v1/discovery/services", server.discoveryServices)
	mux.HandleFunc("GET /v1/discovery/services/{name}", server.discoveryServiceByName)
	mux.HandleFunc("GET /v1/integrations/traefik/config", server.traefikConfig)
	mux.HandleFunc("GET /v1/events", server.listEvents)
	mux.HandleFunc("GET /v1/logs", server.streamLogs)

	return server.middleware(mux)
}

type HealthResponse struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type ErrorResponse struct {
	Error RequestError `json:"error"`
}

type RequestError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
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
}

type AgentHeartbeatRequest struct {
	NodeID      types.NodeID      `json:"node_id"`
	Capacity    types.Resources   `json:"capacity"`
	Allocatable types.Resources   `json:"allocatable"`
	Labels      map[string]string `json:"labels,omitempty"`
	Shutdown    bool              `json:"shutdown,omitempty"`
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
}

type NodeResponse struct {
	Node types.Node `json:"node"`
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

type requestIDKey struct{}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Time: time.Now().UTC()})
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ready", Time: time.Now().UTC()})
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
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	credential, err := s.issueAgentCredential(r.Context(), command.Node.ID)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
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
		NodeID:      req.NodeID,
		Capacity:    req.Capacity,
		Allocatable: req.Allocatable,
		Labels:      req.Labels,
		Shutdown:    req.Shutdown,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	credential, err := s.issueAgentCredential(r.Context(), command.Node.ID)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
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
	id, ok := s.pathNodeID(w, r)
	if !ok {
		return
	}
	node, err := s.controlPlane.DrainNode(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, NodeResponse{Node: node})
}

func (s *Server) uncordonNode(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathNodeID(w, r)
	if !ok {
		return
	}
	node, err := s.controlPlane.UncordonNode(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, NodeResponse{Node: node})
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
		s.writeError(w, r, err)
		return
	}
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
		s.writeError(w, r, err)
		return
	}
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
		s.writeError(w, r, err)
		return
	}
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
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var req CreateServiceRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	normalized, err := types.NormalizeServiceSpec(req.Spec, types.DefaultResourceDefaults())
	if err != nil {
		s.writeError(w, r, fmt.Errorf("%w: %v", store.ErrInvalidState, err))
		return
	}
	req.Spec = normalized
	if err := req.Spec.Validate(); err != nil {
		s.writeError(w, r, fmt.Errorf("%w: %v", store.ErrInvalidState, err))
		return
	}
	service, err := s.controlPlane.CreateService(r.Context(), req.Spec)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
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
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) scaleService(w http.ResponseWriter, r *http.Request) {
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
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ServiceResponse{Service: service})
}

func (s *Server) rolloutService(w http.ResponseWriter, r *http.Request) {
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
		s.writeError(w, r, err)
		return
	}
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
	id, ok := s.pathServiceID(w, r)
	if !ok {
		return
	}
	deployment, err := s.controlPlane.RollbackService(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
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
	writeJSON(w, http.StatusOK, TraefikConfigResponse(traefik.BuildConfig(services, endpointSets)))
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
	return requestIDMiddleware(s.accessLogMiddleware(s.recoveryMiddleware(s.timeoutMiddleware(s.userAuthMiddleware(next)))))
}

func (s *Server) userAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}
		if !auth.HasRole(principal.Role, required) {
			s.writeAuthError(w, r, http.StatusForbidden, "forbidden", "insufficient role")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

func requiredRole(r *http.Request) (auth.Role, bool) {
	path := r.URL.Path
	if path == "/healthz" || path == "/readyz" || path == "/metrics" || strings.HasPrefix(path, "/v1/agent/") {
		return "", false
	}
	if r.Method == http.MethodGet {
		switch {
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
		case path == "/v1/logs":
			return auth.RoleViewer, true
		case strings.HasPrefix(path, "/v1/rollouts/"):
			return auth.RoleViewer, true
		}
	}
	if r.Method == http.MethodPost {
		switch {
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

func metricRoute(method string, path string) (string, bool) {
	if path == "/metrics" {
		return "", false
	}
	switch {
	case path == "/healthz":
		return "/healthz", true
	case path == "/readyz":
		return "/readyz", true
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
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{
			Error: RequestError{
				Code:      "unauthorized",
				Message:   "invalid agent registration token",
				RequestID: requestID(r.Context()),
			},
		})
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
	status, code := errorStatus(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, ErrorResponse{
		Error: RequestError{
			Code:      code,
			Message:   message,
			RequestID: requestID(r.Context()),
		},
	})
}

func errorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, store.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, store.ErrInvalidState):
		return http.StatusBadRequest, "invalid_request"
	case errors.Is(err, store.ErrDuplicate):
		return http.StatusConflict, "duplicate"
	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout, "request_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "deadline_exceeded"
	default:
		return http.StatusInternalServerError, "internal"
	}
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

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
