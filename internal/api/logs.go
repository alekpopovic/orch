package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type LogStreamRequest struct {
	AgentURL string
	TaskID   string
	Follow   bool
	Tail     string
	Token    string
}

type LogStreamer interface {
	StreamLogs(ctx context.Context, req LogStreamRequest, w io.Writer) error
}

type AgentHTTPLogStreamer struct {
	Client *http.Client
}

func (s *AgentHTTPLogStreamer) StreamLogs(ctx context.Context, req LogStreamRequest, w io.Writer) error {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := normalizeAgentURL(req.AgentURL)
	values := url.Values{}
	values.Set("task_id", req.TaskID)
	if req.Follow {
		values.Set("follow", "true")
	}
	if strings.TrimSpace(req.Tail) != "" {
		values.Set("tail", strings.TrimSpace(req.Tail))
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/agent/logs?"+values.Encode(), nil)
	if err != nil {
		return fmt.Errorf("create agent logs request: %w", err)
	}
	if req.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.Token)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request agent logs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("agent returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("copy agent logs: %w", err)
	}
	return nil
}

func normalizeAgentURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return "http://" + raw
}
