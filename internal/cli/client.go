package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewAPIClient(serverURL string) (*APIClient, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return nil, fmt.Errorf("server URL is required; pass --server, set ORCH_SERVER_URL, or configure the CLI")
	}
	if _, err := url.ParseRequestURI(serverURL); err != nil {
		return nil, fmt.Errorf("invalid server URL %q: %w", serverURL, err)
	}
	return &APIClient{
		baseURL: serverURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *APIClient) ListNodes(ctx context.Context) ([]types.Node, error) {
	var out api.ListNodesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/nodes", nil, &out); err != nil {
		return nil, err
	}
	return out.Nodes, nil
}

func (c *APIClient) GetNode(ctx context.Context, id string) (types.Node, error) {
	var out api.NodeResponse
	if err := c.do(ctx, http.MethodGet, "/v1/nodes/"+id, nil, &out); err != nil {
		return types.Node{}, err
	}
	return out.Node, nil
}

func (c *APIClient) DrainNode(ctx context.Context, id string) (types.Node, error) {
	var out api.NodeResponse
	if err := c.do(ctx, http.MethodPost, "/v1/nodes/"+id+"/drain", nil, &out); err != nil {
		return types.Node{}, err
	}
	return out.Node, nil
}

func (c *APIClient) UncordonNode(ctx context.Context, id string) (types.Node, error) {
	var out api.NodeResponse
	if err := c.do(ctx, http.MethodPost, "/v1/nodes/"+id+"/uncordon", nil, &out); err != nil {
		return types.Node{}, err
	}
	return out.Node, nil
}

func (c *APIClient) CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error) {
	var out api.ServiceResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services", api.CreateServiceRequest{Spec: spec}, &out); err != nil {
		return types.Service{}, err
	}
	return out.Service, nil
}

func (c *APIClient) ListServices(ctx context.Context) ([]types.Service, error) {
	var out api.ListServicesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/services", nil, &out); err != nil {
		return nil, err
	}
	return out.Services, nil
}

func (c *APIClient) GetService(ctx context.Context, id string) (types.Service, error) {
	var out api.ServiceResponse
	if err := c.do(ctx, http.MethodGet, "/v1/services/"+id, nil, &out); err != nil {
		return types.Service{}, err
	}
	return out.Service, nil
}

func (c *APIClient) DeleteService(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/services/"+id, nil, nil)
}

func (c *APIClient) ScaleService(ctx context.Context, id string, replicas int) (types.Service, error) {
	var out api.ServiceResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services/"+id+"/scale", api.ScaleServiceRequest{Replicas: replicas}, &out); err != nil {
		return types.Service{}, err
	}
	return out.Service, nil
}

func (c *APIClient) RolloutService(ctx context.Context, id string, image string, maxUnavailable int, maxSurge int) (types.Deployment, error) {
	var out api.DeploymentResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services/"+id+"/rollout", api.RolloutServiceRequest{
		Image:          image,
		MaxUnavailable: maxUnavailable,
		MaxSurge:       maxSurge,
	}, &out); err != nil {
		return types.Deployment{}, err
	}
	return out.Deployment, nil
}

func (c *APIClient) GetServiceRollout(ctx context.Context, id string) (types.Deployment, error) {
	var out api.DeploymentResponse
	if err := c.do(ctx, http.MethodGet, "/v1/services/"+id+"/rollout", nil, &out); err != nil {
		return types.Deployment{}, err
	}
	return out.Deployment, nil
}

func (c *APIClient) RollbackService(ctx context.Context, id string) (types.Deployment, error) {
	var out api.DeploymentResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services/"+id+"/rollback", nil, &out); err != nil {
		return types.Deployment{}, err
	}
	return out.Deployment, nil
}

func (c *APIClient) ListTasks(ctx context.Context, query url.Values) ([]types.Task, error) {
	path := "/v1/tasks"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out api.ListTasksResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Tasks, nil
}

func (c *APIClient) GetTask(ctx context.Context, id string) (types.Task, error) {
	var out api.TaskResponse
	if err := c.do(ctx, http.MethodGet, "/v1/tasks/"+id, nil, &out); err != nil {
		return types.Task{}, err
	}
	return out.Task, nil
}

func (c *APIClient) ListEvents(ctx context.Context, filter events.Filter) ([]types.Event, error) {
	var out api.ListEventsResponse
	path := "/v1/events"
	if query := eventQuery(filter).Encode(); query != "" {
		path += "?" + query
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Events, nil
}

func eventQuery(filter events.Filter) url.Values {
	values := url.Values{}
	if filter.ServiceID != "" {
		values.Set("service_id", string(filter.ServiceID))
	}
	if filter.TaskID != "" {
		values.Set("task_id", string(filter.TaskID))
	}
	if filter.NodeID != "" {
		values.Set("node_id", string(filter.NodeID))
	}
	if filter.Type != "" {
		values.Set("type", filter.Type)
	}
	if filter.Severity != "" {
		values.Set("severity", string(filter.Severity))
	}
	if !filter.Since.IsZero() {
		values.Set("since", filter.Since.UTC().Format(time.RFC3339Nano))
	}
	if filter.Limit > 0 {
		values.Set("limit", strconv.Itoa(filter.Limit))
	}
	return values
}

func (c *APIClient) StreamLogs(ctx context.Context, serviceID string, taskID string, follow bool, tail string, out io.Writer) error {
	values := url.Values{}
	if strings.TrimSpace(taskID) != "" {
		values.Set("task_id", strings.TrimSpace(taskID))
	} else {
		values.Set("service_id", serviceID)
	}
	if follow {
		values.Set("follow", "true")
	}
	if strings.TrimSpace(tail) != "" {
		values.Set("tail", strings.TrimSpace(tail))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/logs?"+values.Encode(), nil)
	if err != nil {
		return fmt.Errorf("create logs request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request logs failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("stream logs: %w", err)
	}
	return nil
}

func (c *APIClient) do(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
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
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s failed: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func decodeAPIError(resp *http.Response) error {
	var body api.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Error.Message != "" {
		if body.Error.RequestID != "" {
			return fmt.Errorf("%s (request %s)", body.Error.Message, body.Error.RequestID)
		}
		return fmt.Errorf("%s", body.Error.Message)
	}
	return fmt.Errorf("server returned %s", resp.Status)
}
