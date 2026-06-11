package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/config"
	orchdocker "github.com/alekpopovic/orch/internal/docker"
	"github.com/alekpopovic/orch/internal/health"
	"github.com/alekpopovic/orch/pkg/types"
)

type Client interface {
	Register(ctx context.Context, req api.AgentRegisterRequest) (api.AgentResponse, error)
	Heartbeat(ctx context.Context, req api.AgentHeartbeatRequest) (api.AgentResponse, error)
	ListAssignedTasks(ctx context.Context, nodeID types.NodeID) ([]api.AgentTask, error)
	ReportTaskStatus(ctx context.Context, taskID types.TaskID, req api.AgentTaskStatusRequest) (types.Task, error)
}

type Runner struct {
	cfg          config.AgentConfig
	client       Client
	runtime      orchdocker.Runtime
	health       health.Checker
	healthStates map[types.TaskID]healthState
	logger       *slog.Logger
	rand         *rand.Rand
}

type healthState struct {
	successes int
	failures  int
}

func NewRunner(cfg config.AgentConfig, client Client, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		cfg:          cfg,
		client:       client,
		health:       health.NewChecker(),
		healthStates: make(map[types.TaskID]healthState),
		logger:       logger,
		rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *Runner) WithRuntime(runtime orchdocker.Runtime) *Runner {
	r.runtime = runtime
	return r
}

func (r *Runner) WithHealthChecker(checker health.Checker) *Runner {
	r.health = checker
	return r
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.cfg.Validate(); err != nil {
		return err
	}
	if r.client == nil {
		client, err := NewHTTPClient(r.cfg.ServerURL, r.cfg.BootstrapToken)
		if err != nil {
			return err
		}
		r.client = client
	}
	if r.runtime == nil {
		runtime, err := orchdocker.NewEngineRuntimeFromEnv()
		if err != nil {
			return err
		}
		r.runtime = runtime
	}

	capacity := DetectCapacity()
	registerResp, err := r.registerWithRetry(ctx, capacity)
	if err != nil {
		return err
	}
	node := registerResp.Node
	r.logger.Info("agent registered", "node_id", node.ID, "status", node.Status)

	timer := time.NewTimer(r.nextHeartbeatDelay())
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return r.notifyShutdown(node.ID, capacity)
		case <-timer.C:
			resp, err := r.heartbeatWithRetry(ctx, node.ID, capacity, false)
			if err != nil {
				return err
			}
			node = resp.Node
			if err := r.reconcileAssignedTasks(ctx, node.ID); err != nil {
				r.logger.Warn("task reconciliation failed", "node_id", node.ID, "error", err)
			}
			r.logger.Info("agent heartbeat acknowledged", "node_id", node.ID, "status", node.Status)
			timer.Reset(r.nextHeartbeatDelay())
		}
	}
}

func (r *Runner) reconcileAssignedTasks(ctx context.Context, nodeID types.NodeID) error {
	tasks, err := r.client.ListAssignedTasks(ctx, nodeID)
	if err != nil {
		return err
	}
	assigned := make(map[types.TaskID]types.Task, len(tasks))
	for _, assignedTask := range tasks {
		task := assignedTask.Task
		assigned[task.ID] = task
		if err := r.ensureTask(ctx, nodeID, assignedTask); err != nil {
			r.logger.Warn("task execution failed", "task_id", task.ID, "error", err)
		}
	}
	return r.cleanupUnassigned(ctx, nodeID, assigned)
}

func (r *Runner) ensureTask(ctx context.Context, nodeID types.NodeID, assigned api.AgentTask) error {
	task := assigned.Task
	if task.DesiredStatus == types.TaskStopped || task.DesiredStatus == types.TaskRemoved {
		return r.removeAssignedTask(ctx, nodeID, task)
	}
	if task.DesiredStatus != types.TaskRunning {
		return nil
	}
	if task.ContainerID != "" {
		status, err := r.runtime.InspectContainer(ctx, orchdocker.ContainerID(task.ContainerID))
		if err == nil && status.Running {
			return r.checkTaskHealth(ctx, nodeID, assigned, status)
		}
		if err == nil {
			if err := r.runtime.StartContainer(ctx, status.ID); err != nil {
				_, _ = r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskFailed, ContainerID: string(status.ID), FailureReason: err.Error()})
				return err
			}
			_, err = r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskRunning, ContainerID: string(status.ID)})
			if err != nil {
				return err
			}
			assigned.Task.ActualStatus = types.TaskRunning
			assigned.Task.ContainerID = string(status.ID)
			return r.checkTaskHealth(ctx, nodeID, assigned, status)
		}
	}
	if _, err := r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskPulling}); err != nil {
		return err
	}
	if err := r.runtime.PullImage(ctx, task.Image, nil); err != nil {
		_, _ = r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskFailed, FailureReason: err.Error()})
		return err
	}

	containerID, err := r.runtime.CreateContainer(ctx, orchdocker.ContainerSpec{
		Name:      "orch-" + string(task.ID),
		Image:     task.Image,
		ServiceID: string(task.ServiceID),
		TaskID:    string(task.ID),
		NodeID:    string(nodeID),
		Version:   task.Version,
	})
	if err != nil {
		_, _ = r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskFailed, FailureReason: err.Error()})
		return err
	}
	if _, err := r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskCreated, ContainerID: string(containerID)}); err != nil {
		return err
	}

	if err := r.runtime.StartContainer(ctx, containerID); err != nil {
		_, _ = r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskFailed, ContainerID: string(containerID), FailureReason: err.Error()})
		return err
	}
	_, err = r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskRunning, ContainerID: string(containerID)})
	if err != nil {
		return err
	}
	assigned.Task.ContainerID = string(containerID)
	assigned.Task.ActualStatus = types.TaskRunning
	return r.checkTaskHealth(ctx, nodeID, assigned, orchdocker.ContainerStatus{ID: containerID, Running: true})
}

func (r *Runner) removeAssignedTask(ctx context.Context, nodeID types.NodeID, task types.Task) error {
	containerID := orchdocker.ContainerID(task.ContainerID)
	if containerID == "" {
		containers, err := r.runtime.ListManagedContainers(ctx, map[string]string{
			orchdocker.NodeIDLabel: string(nodeID),
			orchdocker.TaskIDLabel: string(task.ID),
		})
		if err != nil {
			return err
		}
		if len(containers) > 0 {
			containerID = containers[0].ID
		}
	}
	if containerID != "" {
		if err := r.runtime.StopContainer(ctx, containerID, 10*time.Second); err != nil {
			return err
		}
		if err := r.runtime.RemoveContainer(ctx, containerID, true); err != nil {
			return err
		}
	}
	_, err := r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{
		NodeID:      nodeID,
		Status:      types.TaskRemoved,
		ContainerID: string(containerID),
	})
	return err
}

func (r *Runner) checkTaskHealth(ctx context.Context, nodeID types.NodeID, assigned api.AgentTask, container orchdocker.ContainerStatus) error {
	task := assigned.Task
	check := assigned.Healthcheck
	if check == nil || check.Type == "" || check.Type == types.HealthcheckNone {
		return nil
	}
	if task.ActualStatus != types.TaskRunning && task.ActualStatus != types.TaskHealthy && task.ActualStatus != types.TaskUnhealthy {
		return nil
	}
	if r.health == nil {
		r.health = health.NewChecker()
	}
	if delay := r.healthJitter(check.Interval); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	probe, ok := healthProbe(*check, assigned.Ports)
	if !ok {
		return nil
	}
	result, err := r.health.Check(ctx, probe)
	if err != nil {
		return err
	}
	state := r.healthStates[task.ID]
	if result.Healthy {
		state.successes++
		state.failures = 0
		r.healthStates[task.ID] = state
		threshold := check.HealthyThreshold
		if threshold <= 0 {
			threshold = 1
		}
		if state.successes >= threshold && task.ActualStatus != types.TaskHealthy {
			_, err = r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskHealthy, ContainerID: string(container.ID)})
			return err
		}
		return nil
	}

	state.failures++
	state.successes = 0
	r.healthStates[task.ID] = state
	threshold := check.UnhealthyThreshold
	if threshold <= 0 {
		threshold = 3
	}
	if state.failures >= threshold {
		_, err = r.client.ReportTaskStatus(ctx, task.ID, api.AgentTaskStatusRequest{
			NodeID:        nodeID,
			Status:        types.TaskUnhealthy,
			ContainerID:   string(container.ID),
			FailureReason: result.Message,
		})
		return err
	}
	return nil
}

func (r *Runner) healthJitter(interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	max := interval / 5
	if max <= 0 {
		return 0
	}
	return time.Duration(r.rand.Int63n(int64(max)))
}

func healthProbe(check types.Healthcheck, ports []types.Port) (health.Check, bool) {
	port := check.Port
	if port == 0 && len(ports) > 0 {
		port = ports[0].PublishedPort
		if port == 0 {
			port = ports[0].ContainerPort
		}
	}
	if port == 0 {
		return health.Check{}, false
	}
	timeout := check.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	target := "127.0.0.1:" + strconv.Itoa(port)
	switch check.Type {
	case types.HealthcheckHTTP:
		path := check.Path
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return health.Check{Type: health.HTTPProbe, Target: "http://" + target + path, Timeout: timeout}, true
	case types.HealthcheckTCP:
		return health.Check{Type: health.TCPProbe, Target: target, Timeout: timeout}, true
	default:
		return health.Check{}, false
	}
}

func (r *Runner) cleanupUnassigned(ctx context.Context, nodeID types.NodeID, assigned map[types.TaskID]types.Task) error {
	containers, err := r.runtime.ListManagedContainers(ctx, map[string]string{orchdocker.NodeIDLabel: string(nodeID)})
	if err != nil {
		return err
	}
	for _, container := range containers {
		taskID := types.TaskID(container.Labels[orchdocker.TaskIDLabel])
		if taskID == "" {
			continue
		}
		if _, ok := assigned[taskID]; ok {
			continue
		}
		if err := r.runtime.StopContainer(ctx, container.ID, 10*time.Second); err != nil {
			r.logger.Warn("failed to stop unassigned container", "container_id", container.ID, "task_id", taskID, "error", err)
			continue
		}
		if err := r.runtime.RemoveContainer(ctx, container.ID, true); err != nil {
			r.logger.Warn("failed to remove unassigned container", "container_id", container.ID, "task_id", taskID, "error", err)
			continue
		}
		_, _ = r.client.ReportTaskStatus(ctx, taskID, api.AgentTaskStatusRequest{NodeID: nodeID, Status: types.TaskRemoved, ContainerID: string(container.ID)})
	}
	return nil
}

func (r *Runner) registerWithRetry(ctx context.Context, capacity types.Resources) (api.AgentResponse, error) {
	var response api.AgentResponse
	err := retry(ctx, r.rand, func() error {
		var err error
		response, err = r.client.Register(ctx, api.AgentRegisterRequest{
			NodeName:         r.cfg.NodeName,
			AdvertiseAddress: r.cfg.AdvertiseAddress,
			Labels:           r.cfg.Labels,
			Capacity:         capacity,
			Allocatable:      capacity,
			DockerSocketPath: r.cfg.DockerSocketPath,
		})
		return err
	})
	return response, err
}

func (r *Runner) heartbeatWithRetry(ctx context.Context, nodeID types.NodeID, capacity types.Resources, shutdown bool) (api.AgentResponse, error) {
	var response api.AgentResponse
	err := retry(ctx, r.rand, func() error {
		var err error
		response, err = r.client.Heartbeat(ctx, api.AgentHeartbeatRequest{
			NodeID:      nodeID,
			Capacity:    capacity,
			Allocatable: capacity,
			Labels:      r.cfg.Labels,
			Shutdown:    shutdown,
		})
		return err
	})
	return response, err
}

func (r *Runner) notifyShutdown(nodeID types.NodeID, capacity types.Resources) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), r.cfg.GracefulShutdownTTL)
	defer cancel()
	_, err := r.heartbeatWithRetry(shutdownCtx, nodeID, capacity, true)
	if err != nil {
		r.logger.Warn("failed to notify shutdown", "node_id", nodeID, "error", err)
		return nil
	}
	r.logger.Info("agent shutdown notified", "node_id", nodeID)
	return nil
}

func (r *Runner) nextHeartbeatDelay() time.Duration {
	interval := r.cfg.HeartbeatInterval
	jitterRange := interval / 5
	if jitterRange <= 0 {
		return interval
	}
	jitter := time.Duration(r.rand.Int63n(int64(jitterRange)))
	return interval - jitterRange/2 + jitter
}

func retry(ctx context.Context, rng *rand.Rand, operation func() error) error {
	backoff := 250 * time.Millisecond
	for {
		err := operation()
		if err == nil {
			return nil
		}
		delay := backoff + time.Duration(rng.Int63n(int64(backoff)))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
}

func DetectCapacity() types.Resources {
	return types.Resources{
		CPU:    int64(runtime.NumCPU() * 1000),
		Memory: detectMemory(),
	}
}

func detectMemory() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kib, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kib * 1024
	}
	return 0
}

type HTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPClient(serverURL string, token string) (*HTTPClient, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	return &HTTPClient{
		baseURL: serverURL,
		token:   token,
		client:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *HTTPClient) Register(ctx context.Context, req api.AgentRegisterRequest) (api.AgentResponse, error) {
	var out api.AgentResponse
	if err := c.do(ctx, "/v1/agent/register", req, &out); err != nil {
		return api.AgentResponse{}, err
	}
	return out, nil
}

func (c *HTTPClient) Heartbeat(ctx context.Context, req api.AgentHeartbeatRequest) (api.AgentResponse, error) {
	var out api.AgentResponse
	if err := c.do(ctx, "/v1/agent/heartbeat", req, &out); err != nil {
		return api.AgentResponse{}, err
	}
	return out, nil
}

func (c *HTTPClient) ListAssignedTasks(ctx context.Context, nodeID types.NodeID) ([]api.AgentTask, error) {
	var out api.AgentTasksResponse
	path := "/v1/agent/tasks?node_id=" + url.QueryEscape(string(nodeID))
	if err := c.doMethod(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Tasks, nil
}

func (c *HTTPClient) ReportTaskStatus(ctx context.Context, taskID types.TaskID, req api.AgentTaskStatusRequest) (types.Task, error) {
	var out api.TaskResponse
	if err := c.do(ctx, "/v1/agent/tasks/"+string(taskID)+"/status", req, &out); err != nil {
		return types.Task{}, err
	}
	return out.Task, nil
}

func (c *HTTPClient) do(ctx context.Context, path string, body any, out any) error {
	return c.doMethod(ctx, http.MethodPost, path, body, out)
}

func (c *HTTPClient) doMethod(ctx context.Context, method string, path string, body any, out any) error {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr api.ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Error.Message != "" {
			return fmt.Errorf("%s", apiErr.Error.Message)
		}
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
