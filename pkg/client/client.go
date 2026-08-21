package client

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

	"github.com/alekpopovic/orch/pkg/types"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	namespace  string
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(client *Client) {
		if httpClient != nil {
			client.httpClient = httpClient
		}
	}
}

func WithBearerToken(token string) Option {
	return func(client *Client) {
		client.token = strings.TrimSpace(token)
	}
}

func WithNamespace(namespace string) Option {
	return func(client *Client) {
		client.namespace = strings.TrimSpace(namespace)
	}
}

func New(serverURL string, options ...Option) (*Client, error) {
	normalized, err := NormalizeServerURL(serverURL)
	if err != nil {
		return nil, err
	}
	client := &Client{
		baseURL: normalized,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, option := range options {
		option(client)
	}
	return client, nil
}

func NormalizeServerURL(serverURL string) (string, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return "", fmt.Errorf("server URL is required")
	}
	parsed, err := url.ParseRequestURI(serverURL)
	if err != nil {
		return "", fmt.Errorf("invalid server URL %q: %w", serverURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid server URL %q: scheme must be http or https", serverURL)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid server URL %q: host is required", serverURL)
	}
	return serverURL, nil
}

func (c *Client) SetToken(token string) {
	c.token = strings.TrimSpace(token)
}

func (c *Client) SetNamespace(namespace string) {
	c.namespace = strings.TrimSpace(namespace)
}

func (c *Client) CreateNamespace(ctx context.Context, name string) (types.Namespace, error) {
	var out namespaceResponse
	if err := c.do(ctx, http.MethodPost, "/v1/namespaces", createNamespaceRequest{Name: name}, &out); err != nil {
		return types.Namespace{}, err
	}
	return out.Namespace, nil
}

func (c *Client) ListNamespaces(ctx context.Context) ([]types.Namespace, error) {
	var out listNamespacesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/namespaces", nil, &out); err != nil {
		return nil, err
	}
	return out.Namespaces, nil
}

func (c *Client) DeleteNamespace(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/namespaces/"+url.PathEscape(name), nil, nil)
}

func (c *Client) GetResourceQuota(ctx context.Context) (types.ResourceQuota, types.ResourceUsage, error) {
	var out resourceQuotaResponse
	if err := c.do(ctx, http.MethodGet, "/v1/quota", nil, &out); err != nil {
		return types.ResourceQuota{}, types.ResourceUsage{}, err
	}
	return out.Quota, out.Usage, nil
}

func (c *Client) SetResourceQuota(ctx context.Context, value types.ResourceQuota) (types.ResourceQuota, types.ResourceUsage, error) {
	var out resourceQuotaResponse
	if err := c.do(ctx, http.MethodPut, "/v1/quota", value, &out); err != nil {
		return types.ResourceQuota{}, types.ResourceUsage{}, err
	}
	return out.Quota, out.Usage, nil
}

func (c *Client) CreateGitOpsSource(ctx context.Context, source types.GitOpsSource) (types.GitOpsSource, error) {
	var out gitOpsSourceResponse
	request := createGitOpsSourceRequest{RepositoryURL: source.RepositoryURL, Branch: source.Branch, Path: source.Path, SyncInterval: source.SyncInterval.String(), Prune: source.Prune}
	if err := c.do(ctx, http.MethodPost, "/v1/gitops/sources", request, &out); err != nil {
		return types.GitOpsSource{}, err
	}
	return out.Source, nil
}

func (c *Client) ListGitOpsSources(ctx context.Context) ([]types.GitOpsSource, error) {
	var out listGitOpsSourcesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/gitops/sources", nil, &out); err != nil {
		return nil, err
	}
	return out.Sources, nil
}

func (c *Client) SyncGitOpsSource(ctx context.Context, id string) (types.GitOpsSource, error) {
	var out gitOpsSourceResponse
	if err := c.do(ctx, http.MethodPost, "/v1/gitops/sources/"+url.PathEscape(id)+"/sync", nil, &out); err != nil {
		return types.GitOpsSource{}, err
	}
	return out.Source, nil
}

func (c *Client) DeleteGitOpsSource(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/gitops/sources/"+url.PathEscape(id), nil, nil)
}

func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var out HealthResponse
	if err := c.do(ctx, http.MethodGet, "/healthz", nil, &out); err != nil {
		return HealthResponse{}, err
	}
	return out, nil
}

func (c *Client) Ready(ctx context.Context) (HealthResponse, error) {
	var out HealthResponse
	if err := c.do(ctx, http.MethodGet, "/readyz", nil, &out); err != nil {
		return HealthResponse{}, err
	}
	return out, nil
}

func (c *Client) ListNodes(ctx context.Context) ([]types.Node, error) {
	var out listNodesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/nodes", nil, &out); err != nil {
		return nil, err
	}
	return out.Nodes, nil
}

func (c *Client) GetNode(ctx context.Context, id string) (types.Node, error) {
	var out nodeResponse
	if err := c.do(ctx, http.MethodGet, "/v1/nodes/"+url.PathEscape(id), nil, &out); err != nil {
		return types.Node{}, err
	}
	return out.Node, nil
}

func (c *Client) DrainNode(ctx context.Context, id string) (types.Node, error) {
	var out nodeResponse
	if err := c.do(ctx, http.MethodPost, "/v1/nodes/"+url.PathEscape(id)+"/drain", nil, &out); err != nil {
		return types.Node{}, err
	}
	return out.Node, nil
}

func (c *Client) UncordonNode(ctx context.Context, id string) (types.Node, error) {
	var out nodeResponse
	if err := c.do(ctx, http.MethodPost, "/v1/nodes/"+url.PathEscape(id)+"/uncordon", nil, &out); err != nil {
		return types.Node{}, err
	}
	return out.Node, nil
}

func (c *Client) GetNodeDrainStatus(ctx context.Context, id string) (NodeDrainStatus, error) {
	var out nodeDrainStatusResponse
	if err := c.do(ctx, http.MethodGet, "/v1/nodes/"+url.PathEscape(id)+"/drain-status", nil, &out); err != nil {
		return NodeDrainStatus{}, err
	}
	return out.DrainStatus, nil
}

func (c *Client) CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error) {
	normalized, err := types.NormalizeServiceSpec(spec, types.DefaultResourceDefaults())
	if err != nil {
		return types.Service{}, fmt.Errorf("normalize service spec: %w", err)
	}
	var out serviceResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services", createServiceRequest{Spec: normalized}, &out); err != nil {
		return types.Service{}, err
	}
	return out.Service, nil
}

func (c *Client) ListServices(ctx context.Context) ([]types.Service, error) {
	var out listServicesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/services", nil, &out); err != nil {
		return nil, err
	}
	return out.Services, nil
}

func (c *Client) GetService(ctx context.Context, id string) (types.Service, error) {
	var out serviceResponse
	if err := c.do(ctx, http.MethodGet, "/v1/services/"+url.PathEscape(id), nil, &out); err != nil {
		return types.Service{}, err
	}
	return out.Service, nil
}

func (c *Client) DeleteService(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/services/"+url.PathEscape(id), nil, nil)
}

func (c *Client) ScaleService(ctx context.Context, id string, replicas int) (types.Service, error) {
	var out serviceResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services/"+url.PathEscape(id)+"/scale", scaleServiceRequest{Replicas: replicas}, &out); err != nil {
		return types.Service{}, err
	}
	return out.Service, nil
}

func (c *Client) RolloutService(ctx context.Context, id string, request RolloutServiceRequest) (types.Deployment, error) {
	var out deploymentResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services/"+url.PathEscape(id)+"/rollout", request, &out); err != nil {
		return types.Deployment{}, err
	}
	return out.Deployment, nil
}

func (c *Client) GetServiceRollout(ctx context.Context, id string) (types.Deployment, error) {
	var out deploymentResponse
	if err := c.do(ctx, http.MethodGet, "/v1/services/"+url.PathEscape(id)+"/rollout", nil, &out); err != nil {
		return types.Deployment{}, err
	}
	return out.Deployment, nil
}

func (c *Client) RollbackService(ctx context.Context, id string) (types.Deployment, error) {
	var out deploymentResponse
	if err := c.do(ctx, http.MethodPost, "/v1/services/"+url.PathEscape(id)+"/rollback", nil, &out); err != nil {
		return types.Deployment{}, err
	}
	return out.Deployment, nil
}

func (c *Client) GetRollout(ctx context.Context, id string) (types.Deployment, error) {
	var out deploymentResponse
	if err := c.do(ctx, http.MethodGet, "/v1/rollouts/"+url.PathEscape(id), nil, &out); err != nil {
		return types.Deployment{}, err
	}
	return out.Deployment, nil
}

func (c *Client) ListTasks(ctx context.Context, filter TaskFilter) ([]types.Task, error) {
	var out listTasksResponse
	path := "/v1/tasks"
	if query := filter.query().Encode(); query != "" {
		path += "?" + query
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Tasks, nil
}

func (c *Client) GetTask(ctx context.Context, id string) (types.Task, error) {
	var out taskResponse
	if err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id), nil, &out); err != nil {
		return types.Task{}, err
	}
	return out.Task, nil
}

func (c *Client) ListEvents(ctx context.Context, filter EventFilter) ([]types.Event, error) {
	var out listEventsResponse
	path := "/v1/events"
	if query := filter.query().Encode(); query != "" {
		path += "?" + query
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Events, nil
}

func (c *Client) StreamLogs(ctx context.Context, request LogStreamRequest, out io.Writer) error {
	path := "/v1/logs"
	if query := request.query().Encode(); query != "" {
		path += "?" + query
	}
	return c.doStream(ctx, http.MethodGet, path, out)
}

func (c *Client) CreateSecret(ctx context.Context, name string, value string) (types.SecretMetadata, error) {
	var out secretResponse
	if err := c.do(ctx, http.MethodPost, "/v1/secrets", createSecretRequest{Name: name, Value: value}, &out); err != nil {
		return types.SecretMetadata{}, err
	}
	return out.Secret, nil
}

func (c *Client) ListSecrets(ctx context.Context) ([]types.SecretMetadata, error) {
	var out listSecretsResponse
	if err := c.do(ctx, http.MethodGet, "/v1/secrets", nil, &out); err != nil {
		return nil, err
	}
	return out.Secrets, nil
}

func (c *Client) GetSecret(ctx context.Context, name string) (types.SecretMetadata, error) {
	var out secretResponse
	if err := c.do(ctx, http.MethodGet, "/v1/secrets/"+url.PathEscape(name), nil, &out); err != nil {
		return types.SecretMetadata{}, err
	}
	return out.Secret, nil
}

func (c *Client) DeleteSecret(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/secrets/"+url.PathEscape(name), nil, nil)
}

func (c *Client) CreateRegistryCredential(ctx context.Context, request CreateRegistryCredentialRequest) (types.RegistryCredentialMetadata, error) {
	var out registryCredentialResponse
	if err := c.do(ctx, http.MethodPost, "/v1/registry-credentials", request, &out); err != nil {
		return types.RegistryCredentialMetadata{}, err
	}
	return out.Credential, nil
}

func (c *Client) ListRegistryCredentials(ctx context.Context) ([]types.RegistryCredentialMetadata, error) {
	var out listRegistryCredentialsResponse
	if err := c.do(ctx, http.MethodGet, "/v1/registry-credentials", nil, &out); err != nil {
		return nil, err
	}
	return out.Credentials, nil
}

func (c *Client) DeleteRegistryCredential(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/registry-credentials/"+url.PathEscape(id), nil, nil)
}

func (c *Client) ListAuditLogs(ctx context.Context, filter AuditFilter) ([]AuditLog, error) {
	var out listAuditLogsResponse
	path := "/v1/audit"
	if query := filter.query().Encode(); query != "" {
		path += "?" + query
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.AuditLogs, nil
}

func (c *Client) GetServiceEndpoints(ctx context.Context, id string, includeUnhealthy bool) (ServiceEndpoints, error) {
	path := "/v1/services/" + url.PathEscape(id) + "/endpoints"
	if includeUnhealthy {
		path += "?include_unhealthy=true"
	}
	var out ServiceEndpoints
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return ServiceEndpoints{}, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method string, path string, body any, out any) error {
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
	c.authorize(req)

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

func (c *Client) doStream(ctx context.Context, method string, path string, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.authorize(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s failed: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("stream response: %w", err)
	}
	return nil
}

func (c *Client) authorize(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Orch-Namespace", c.namespace)
	}
}

func decodeAPIError(resp *http.Response) error {
	var body ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Error.Message != "" {
		return &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Code:       body.Error.Code,
			Message:    body.Error.Message,
			RequestID:  body.Error.RequestID,
			Details:    body.Error.Details,
		}
	}
	return &APIError{StatusCode: resp.StatusCode, Status: resp.Status}
}

func setString(values url.Values, key string, value string) {
	if value != "" {
		values.Set(key, value)
	}
}

func setTime(values url.Values, key string, value time.Time) {
	if !value.IsZero() {
		values.Set(key, value.UTC().Format(time.RFC3339Nano))
	}
}

func setLimit(values url.Values, limit int) {
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
}
