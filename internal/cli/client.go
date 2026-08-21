package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/apperrors"
	"github.com/alekpopovic/orch/internal/audit"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/discovery"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/internal/gitops"
	orchclient "github.com/alekpopovic/orch/pkg/client"
	"github.com/alekpopovic/orch/pkg/types"
)

type APIClient struct {
	baseURL    string
	httpClient *http.Client
	token      string
	namespace  string
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

func NewAPIClient(serverURL string) (*APIClient, error) {
	normalized, err := orchclient.NormalizeServerURL(serverURL)
	if err != nil && strings.TrimSpace(serverURL) == "" {
		return nil, fmt.Errorf("server URL is required; pass --server, set ORCH_SERVER_URL, or configure the CLI")
	}
	if err != nil {
		return nil, fmt.Errorf("invalid server URL %q: %w", serverURL, err)
	}
	return &APIClient{
		baseURL: normalized,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *APIClient) SetToken(token string) {
	c.token = strings.TrimSpace(token)
}

func (c *APIClient) SetNamespace(namespace string) {
	c.namespace = strings.TrimSpace(namespace)
}

func (c *APIClient) CreateNamespace(ctx context.Context, name string) (types.Namespace, error) {
	var out api.NamespaceResponse
	if err := c.do(ctx, http.MethodPost, "/v1/namespaces", api.CreateNamespaceRequest{Name: name}, &out); err != nil {
		return types.Namespace{}, err
	}
	return out.Namespace, nil
}

func (c *APIClient) ListNamespaces(ctx context.Context) ([]types.Namespace, error) {
	var out api.ListNamespacesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/namespaces", nil, &out); err != nil {
		return nil, err
	}
	return out.Namespaces, nil
}

func (c *APIClient) DeleteNamespace(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/namespaces/"+url.PathEscape(name), nil, nil)
}

func (c *APIClient) GetResourceQuota(ctx context.Context) (types.ResourceQuota, types.ResourceUsage, error) {
	var out api.ResourceQuotaResponse
	if err := c.do(ctx, http.MethodGet, "/v1/quota", nil, &out); err != nil {
		return types.ResourceQuota{}, types.ResourceUsage{}, err
	}
	return out.Quota, out.Usage, nil
}

func (c *APIClient) SetResourceQuota(ctx context.Context, value types.ResourceQuota) (types.ResourceQuota, types.ResourceUsage, error) {
	var out api.ResourceQuotaResponse
	if err := c.do(ctx, http.MethodPut, "/v1/quota", value, &out); err != nil {
		return types.ResourceQuota{}, types.ResourceUsage{}, err
	}
	return out.Quota, out.Usage, nil
}

func (c *APIClient) CreateGitOpsSource(ctx context.Context, source types.GitOpsSource) (types.GitOpsSource, error) {
	var out api.GitOpsSourceResponse
	request := api.CreateGitOpsSourceRequest{RepositoryURL: source.RepositoryURL, Branch: source.Branch, Path: source.Path, SyncInterval: source.SyncInterval.String(), Prune: source.Prune, DriftPolicy: source.DriftPolicy}
	if err := c.do(ctx, http.MethodPost, "/v1/gitops/sources", request, &out); err != nil {
		return types.GitOpsSource{}, err
	}
	return out.Source, nil
}

func (c *APIClient) ListGitOpsSources(ctx context.Context) ([]types.GitOpsSource, error) {
	var out api.ListGitOpsSourcesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/gitops/sources", nil, &out); err != nil {
		return nil, err
	}
	return out.Sources, nil
}

func (c *APIClient) SyncGitOpsSource(ctx context.Context, id string) (types.GitOpsSource, error) {
	var out api.GitOpsSourceResponse
	if err := c.do(ctx, http.MethodPost, "/v1/gitops/sources/"+url.PathEscape(id)+"/sync", nil, &out); err != nil {
		return types.GitOpsSource{}, err
	}
	return out.Source, nil
}

func (c *APIClient) DeleteGitOpsSource(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/gitops/sources/"+url.PathEscape(id), nil, nil)
}

func (c *APIClient) GitOpsStatus(ctx context.Context) ([]types.Service, error) {
	var out api.ListServicesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/gitops/status", nil, &out); err != nil {
		return nil, err
	}
	return out.Services, nil
}
func (c *APIClient) GitOpsDiff(ctx context.Context, name string) (gitops.Diff, error) {
	var out gitops.Diff
	err := c.do(ctx, http.MethodGet, "/v1/gitops/diff/"+url.PathEscape(name), nil, &out)
	return out, err
}
func (c *APIClient) CreateJob(ctx context.Context, v types.JobSpec) (types.Job, error) {
	var out api.JobResponse
	err := c.do(ctx, http.MethodPost, "/v1/jobs", v, &out)
	return out.Job, err
}
func (c *APIClient) ListJobs(ctx context.Context) ([]types.Job, error) {
	var out api.JobsResponse
	err := c.do(ctx, http.MethodGet, "/v1/jobs", nil, &out)
	return out.Jobs, err
}
func (c *APIClient) GetJob(ctx context.Context, id string) (types.Job, error) {
	var out api.JobResponse
	err := c.do(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(id), nil, &out)
	return out.Job, err
}
func (c *APIClient) DeleteJob(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/jobs/"+url.PathEscape(id), nil, nil)
}
func (c *APIClient) CreateCronJob(ctx context.Context, v types.CronJobSpec) (types.CronJob, error) {
	var out api.CronJobResponse
	err := c.do(ctx, http.MethodPost, "/v1/cronjobs", v, &out)
	return out.CronJob, err
}
func (c *APIClient) ListCronJobs(ctx context.Context) ([]types.CronJob, error) {
	var out api.CronJobsResponse
	err := c.do(ctx, http.MethodGet, "/v1/cronjobs", nil, &out)
	return out.CronJobs, err
}
func (c *APIClient) SetCronJobSuspended(ctx context.Context, id string, value bool) (types.CronJob, error) {
	action := "resume"
	if value {
		action = "suspend"
	}
	var out api.CronJobResponse
	err := c.do(ctx, http.MethodPost, "/v1/cronjobs/"+url.PathEscape(id)+"/"+action, nil, &out)
	return out.CronJob, err
}
func (c *APIClient) CreateVolume(ctx context.Context, v types.Volume) (types.Volume, error) {
	var out api.VolumeResponse
	err := c.do(ctx, http.MethodPost, "/v1/volumes", v, &out)
	return out.Volume, err
}
func (c *APIClient) ListVolumes(ctx context.Context) ([]types.Volume, error) {
	var out api.VolumesResponse
	err := c.do(ctx, http.MethodGet, "/v1/volumes", nil, &out)
	return out.Volumes, err
}
func (c *APIClient) GetVolume(ctx context.Context, id string) (types.Volume, error) {
	var out api.VolumeResponse
	err := c.do(ctx, http.MethodGet, "/v1/volumes/"+url.PathEscape(id), nil, &out)
	return out.Volume, err
}
func (c *APIClient) CreateMaintenanceWindow(ctx context.Context, v types.MaintenanceWindow) (types.MaintenanceWindow, error) {
	var out api.MaintenanceWindowResponse
	err := c.do(ctx, http.MethodPost, "/v1/maintenance-windows", v, &out)
	return out.Window, err
}
func (c *APIClient) ListMaintenanceWindows(ctx context.Context) ([]types.MaintenanceWindow, error) {
	var out api.MaintenanceWindowsResponse
	err := c.do(ctx, http.MethodGet, "/v1/maintenance-windows", nil, &out)
	return out.Windows, err
}
func (c *APIClient) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/maintenance-windows/"+url.PathEscape(id), nil, nil)
}
func (c *APIClient) GetRetentionStatus(ctx context.Context) (types.RetentionStatus, error) {
	var out types.RetentionStatus
	err := c.do(ctx, http.MethodGet, "/v1/retention", nil, &out)
	return out, err
}
func (c *APIClient) PruneRetention(ctx context.Context, dry bool) (types.PruneResult, error) {
	var out types.PruneResult
	path := "/v1/retention/prune"
	if dry {
		path += "?dry_run=true"
	}
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}
func (c *APIClient) GetUsageReport(ctx context.Context, ns string, from, to time.Time) (types.UsageReport, error) {
	query := url.Values{}
	if ns != "" {
		query.Set("namespace", ns)
	}
	if !from.IsZero() {
		query.Set("from", from.Format(time.RFC3339))
	}
	if !to.IsZero() {
		query.Set("to", to.Format(time.RFC3339))
	}
	path := "/v1/usage"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var out types.UsageReport
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
func (c *APIClient) GetVersion(ctx context.Context) (types.VersionInfo, error) {
	var out types.VersionInfo
	err := c.do(ctx, http.MethodGet, "/v1/version", nil, &out)
	return out, err
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
	return c.DrainNodeForce(ctx, id, false)
}

func (c *APIClient) DrainNodeForce(ctx context.Context, id string, force bool) (types.Node, error) {
	var out api.NodeResponse
	path := "/v1/nodes/" + id + "/drain"
	if force {
		path += "?force=true"
	}
	if err := c.do(ctx, http.MethodPost, path, nil, &out); err != nil {
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

func (c *APIClient) GetNodeDrainStatus(ctx context.Context, id string) (controlplane.NodeDrainStatus, error) {
	var out api.NodeDrainStatusResponse
	if err := c.do(ctx, http.MethodGet, "/v1/nodes/"+id+"/drain-status", nil, &out); err != nil {
		return controlplane.NodeDrainStatus{}, err
	}
	return out.DrainStatus, nil
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

func (c *APIClient) GetServiceEndpoints(ctx context.Context, id string, includeUnhealthy bool) (discovery.ServiceEndpoints, error) {
	path := "/v1/services/" + id + "/endpoints"
	if includeUnhealthy {
		path += "?include_unhealthy=true"
	}
	var out api.ServiceEndpointsResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return discovery.ServiceEndpoints{}, err
	}
	return discovery.ServiceEndpoints(out), nil
}

func (c *APIClient) DeleteService(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/services/"+id, nil, nil)
}

func (c *APIClient) ScaleService(ctx context.Context, id string, replicas int) (types.Service, error) {
	return c.ScaleServiceForce(ctx, id, replicas, false)
}

func (c *APIClient) ScaleServiceForce(ctx context.Context, id string, replicas int, force bool) (types.Service, error) {
	var out api.ServiceResponse
	path := "/v1/services/" + id + "/scale"
	if force {
		path += "?force=true"
	}
	if err := c.do(ctx, http.MethodPost, path, api.ScaleServiceRequest{Replicas: replicas}, &out); err != nil {
		return types.Service{}, err
	}
	return out.Service, nil
}

func (c *APIClient) RolloutService(ctx context.Context, id string, image string, maxUnavailable int, maxSurge int) (types.Deployment, error) {
	return c.RolloutServiceForce(ctx, id, image, maxUnavailable, maxSurge, false)
}

func (c *APIClient) RolloutServiceForce(ctx context.Context, id string, image string, maxUnavailable int, maxSurge int, force bool) (types.Deployment, error) {
	var out api.DeploymentResponse
	path := "/v1/services/" + id + "/rollout"
	if force {
		path += "?force=true"
	}
	if err := c.do(ctx, http.MethodPost, path, api.RolloutServiceRequest{
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
	return c.RollbackServiceForce(ctx, id, false)
}

func (c *APIClient) RollbackServiceForce(ctx context.Context, id string, force bool) (types.Deployment, error) {
	var out api.DeploymentResponse
	path := "/v1/services/" + id + "/rollback"
	if force {
		path += "?force=true"
	}
	if err := c.do(ctx, http.MethodPost, path, nil, &out); err != nil {
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

func (c *APIClient) ListAuditLogs(ctx context.Context, filter audit.Filter) ([]audit.Log, error) {
	var out api.ListAuditLogsResponse
	path := "/v1/audit"
	if query := auditQuery(filter).Encode(); query != "" {
		path += "?" + query
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.AuditLogs, nil
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

func auditQuery(filter audit.Filter) url.Values {
	values := url.Values{}
	if filter.ActorType != "" {
		values.Set("actor_type", string(filter.ActorType))
	}
	if filter.ActorID != "" {
		values.Set("actor_id", filter.ActorID)
	}
	if filter.Action != "" {
		values.Set("action", filter.Action)
	}
	if filter.TargetType != "" {
		values.Set("target_type", filter.TargetType)
	}
	if filter.TargetID != "" {
		values.Set("target_id", filter.TargetID)
	}
	if filter.Outcome != "" {
		values.Set("outcome", string(filter.Outcome))
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Orch-Namespace", c.namespace)
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Orch-Namespace", c.namespace)
	}

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
		return &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Code:       body.Error.Code,
			Message:    body.Error.Message,
			RequestID:  body.Error.RequestID,
			Details:    apperrors.RedactDetails(body.Error.Details),
		}
	}
	return &APIError{StatusCode: resp.StatusCode, Status: resp.Status}
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
