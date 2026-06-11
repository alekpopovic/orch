package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/api"
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

func (c *APIClient) RolloutService(ctx context.Context, id string, image string) (types.Deployment, error) {
	var out api.DeploymentResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services/"+id+"/rollout", api.RolloutServiceRequest{Image: image}, &out); err != nil {
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

func (c *APIClient) ListEvents(ctx context.Context) ([]types.Event, error) {
	var out api.ListEventsResponse
	if err := c.do(ctx, http.MethodGet, "/v1/events", nil, &out); err != nil {
		return nil, err
	}
	return out.Events, nil
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
