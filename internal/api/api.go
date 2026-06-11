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

	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
)

type Server struct {
	controlPlane controlplane.Service
	logger       *slog.Logger
	timeout      time.Duration
}

type Option func(*Server)

func WithTimeout(timeout time.Duration) Option {
	return func(server *Server) {
		server.timeout = timeout
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
	}
	for _, opt := range opts {
		opt(server)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.healthz)
	mux.HandleFunc("GET /readyz", server.readyz)
	mux.HandleFunc("GET /v1/nodes", server.listNodes)
	mux.HandleFunc("GET /v1/nodes/{id}", server.getNode)
	mux.HandleFunc("POST /v1/nodes/{id}/drain", server.drainNode)
	mux.HandleFunc("POST /v1/nodes/{id}/uncordon", server.uncordonNode)
	mux.HandleFunc("POST /v1/services", server.createService)
	mux.HandleFunc("GET /v1/services", server.listServices)
	mux.HandleFunc("GET /v1/services/{id}", server.getService)
	mux.HandleFunc("DELETE /v1/services/{id}", server.deleteService)
	mux.HandleFunc("POST /v1/services/{id}/scale", server.scaleService)
	mux.HandleFunc("POST /v1/services/{id}/rollout", server.rolloutService)
	mux.HandleFunc("POST /v1/services/{id}/rollback", server.rollbackService)
	mux.HandleFunc("GET /v1/tasks", server.listTasks)
	mux.HandleFunc("GET /v1/tasks/{id}", server.getTask)
	mux.HandleFunc("GET /v1/events", server.listEvents)

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

type NodeResponse struct {
	Node types.Node `json:"node"`
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

type ScaleServiceRequest struct {
	Replicas int `json:"replicas"`
}

type RolloutServiceRequest struct {
	Image string `json:"image"`
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
	deployment, err := s.controlPlane.RolloutService(r.Context(), id, req.Image)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, DeploymentResponse{Deployment: deployment})
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

func (s *Server) middleware(next http.Handler) http.Handler {
	return requestIDMiddleware(s.accessLogMiddleware(s.recoveryMiddleware(s.timeoutMiddleware(authPlaceholderMiddleware(next)))))
}

func authPlaceholderMiddleware(next http.Handler) http.Handler {
	return next
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
		s.logger.Info("http request",
			"request_id", requestID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
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
		if !validTaskStatus(types.TaskStatus(status)) {
			s.writeError(w, r, fmt.Errorf("%w: status is invalid", store.ErrInvalidState))
			return controlplane.TaskFilter{}, false
		}
		filter.Status = types.TaskStatus(status)
	}
	return filter, true
}

func (s *Server) eventFilter(w http.ResponseWriter, r *http.Request) (controlplane.EventFilter, bool) {
	query := r.URL.Query()
	limit := 100
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 500 {
			s.writeError(w, r, fmt.Errorf("%w: limit must be between 1 and 500", store.ErrInvalidState))
			return controlplane.EventFilter{}, false
		}
		limit = parsed
	}
	objectID := strings.TrimSpace(query.Get("related_object_id"))
	if objectID != "" && !validUUID(objectID) {
		s.writeError(w, r, fmt.Errorf("%w: related_object_id must be a UUID", store.ErrInvalidState))
		return controlplane.EventFilter{}, false
	}
	return controlplane.EventFilter{
		RelatedObjectType: strings.TrimSpace(query.Get("related_object_type")),
		RelatedObjectID:   objectID,
		Limit:             limit,
	}, true
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

func validTaskStatus(status types.TaskStatus) bool {
	switch status {
	case types.TaskPending,
		types.TaskAssigned,
		types.TaskPulling,
		types.TaskStarting,
		types.TaskRunning,
		types.TaskStopping,
		types.TaskStopped,
		types.TaskFailed:
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
