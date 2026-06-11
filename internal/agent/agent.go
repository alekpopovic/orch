package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/config"
	"github.com/alekpopovic/orch/pkg/types"
)

type Client interface {
	Register(ctx context.Context, req api.AgentRegisterRequest) (api.AgentResponse, error)
	Heartbeat(ctx context.Context, req api.AgentHeartbeatRequest) (api.AgentResponse, error)
}

type Runner struct {
	cfg    config.AgentConfig
	client Client
	logger *slog.Logger
	rand   *rand.Rand
}

func NewRunner(cfg config.AgentConfig, client Client, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		cfg:    cfg,
		client: client,
		logger: logger,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
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
			r.logger.Info("agent heartbeat acknowledged", "node_id", node.ID, "status", node.Status)
			timer.Reset(r.nextHeartbeatDelay())
		}
	}
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

func (c *HTTPClient) do(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
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
