package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	orchdocker "github.com/alekpopovic/orch/internal/docker"
)

type LogHandlerOption func(*logServer)

func WithMetricsHandler(handler http.Handler) LogHandlerOption {
	return func(server *logServer) {
		server.metricsHandler = handler
	}
}

func NewLogHandler(runtime orchdocker.Runtime, token string, logger *slog.Logger, opts ...LogHandlerOption) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	server := &logServer{runtime: runtime, token: token, logger: logger}
	for _, opt := range opts {
		opt(server)
	}
	mux := http.NewServeMux()
	if server.metricsHandler != nil {
		mux.Handle("GET /metrics", server.metricsHandler)
	}
	mux.HandleFunc("GET /v1/agent/logs", server.streamLogs)
	return mux
}

type logServer struct {
	runtime        orchdocker.Runtime
	token          string
	logger         *slog.Logger
	metricsHandler http.Handler
}

func (s *logServer) streamLogs(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	if s.runtime == nil {
		http.Error(w, "runtime is not configured", http.StatusServiceUnavailable)
		return
	}
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}
	container, err := s.containerForTask(r.Context(), taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, _ := w.(http.Flusher)

	lines, errs := s.runtime.StreamLogs(r.Context(), container.ID, orchdocker.LogOptions{
		Follow: r.URL.Query().Get("follow") == "true",
		Tail:   tailValue(r.URL.Query().Get("tail")),
		Stdout: true,
		Stderr: true,
	})
	for {
		select {
		case <-r.Context().Done():
			return
		case line, ok := <-lines:
			if !ok {
				if err := <-errs; err != nil {
					s.logger.Warn("docker log stream failed", "task_id", taskID, "error", err)
				}
				return
			}
			if _, err := fmt.Fprintln(w, line.Line); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func (s *logServer) authorize(w http.ResponseWriter, r *http.Request) bool {
	if s.token == "" {
		return true
	}
	token := strings.TrimSpace(r.Header.Get("X-Orch-Bootstrap-Token"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if token != s.token {
		http.Error(w, "invalid agent registration token", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *logServer) containerForTask(ctx context.Context, taskID string) (orchdocker.ContainerStatus, error) {
	containers, err := s.runtime.ListManagedContainers(ctx, map[string]string{orchdocker.TaskIDLabel: taskID})
	if err != nil {
		return orchdocker.ContainerStatus{}, err
	}
	if len(containers) == 0 {
		return orchdocker.ContainerStatus{}, fmt.Errorf("managed container for task %s not found", taskID)
	}
	return containers[0], nil
}

func tailValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "all"
	}
	if _, err := strconv.Atoi(raw); err != nil {
		return "all"
	}
	return raw
}
