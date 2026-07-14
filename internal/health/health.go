package health

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ProbeType string

const (
	NoneProbe ProbeType = "none"
	HTTPProbe ProbeType = "http"
	TCPProbe  ProbeType = "tcp"

	MaxHTTPResponseBodyBytes int64 = 64 * 1024
)

type Check struct {
	Type    ProbeType
	Target  string
	Timeout time.Duration
}

type Result struct {
	Healthy   bool
	CheckedAt time.Time
	Message   string
}

type Checker interface {
	Check(ctx context.Context, check Check) (Result, error)
}

type DefaultChecker struct {
	now func() time.Time
}

func NewChecker() *DefaultChecker {
	return &DefaultChecker{now: func() time.Time { return time.Now().UTC() }}
}

func (c *DefaultChecker) Check(ctx context.Context, check Check) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	timeout := check.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	switch check.Type {
	case "", NoneProbe:
		return Result{Healthy: true, CheckedAt: c.now(), Message: "healthcheck disabled"}, nil
	case HTTPProbe:
		return c.checkHTTP(ctx, check.Target, timeout)
	case TCPProbe:
		return c.checkTCP(ctx, check.Target, timeout)
	default:
		return Result{}, fmt.Errorf("unsupported healthcheck type %q", check.Type)
	}
}

func (c *DefaultChecker) checkHTTP(ctx context.Context, target string, timeout time.Duration) (Result, error) {
	if strings.TrimSpace(target) == "" {
		return Result{}, fmt.Errorf("http healthcheck target is required")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create healthcheck request: %w", err)
	}
	client := &http.Client{
		Timeout:       timeout,
		CheckRedirect: sameOriginRedirect(req.URL),
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Healthy: false, CheckedAt: c.now(), Message: err.Error()}, nil
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, MaxHTTPResponseBodyBytes)); err != nil {
		return Result{Healthy: false, CheckedAt: c.now(), Message: err.Error()}, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return Result{Healthy: false, CheckedAt: c.now(), Message: resp.Status}, nil
	}
	return Result{Healthy: true, CheckedAt: c.now(), Message: resp.Status}, nil
}

func sameOriginRedirect(origin *url.URL) func(*http.Request, []*http.Request) error {
	originScheme := strings.ToLower(origin.Scheme)
	originHost := strings.ToLower(origin.Host)
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if strings.ToLower(req.URL.Scheme) != originScheme || strings.ToLower(req.URL.Host) != originHost {
			return fmt.Errorf("healthcheck redirect blocked: %s", req.URL.Redacted())
		}
		return nil
	}
}

func (c *DefaultChecker) checkTCP(ctx context.Context, target string, timeout time.Duration) (Result, error) {
	if strings.TrimSpace(target) == "" {
		return Result{}, fmt.Errorf("tcp healthcheck target is required")
	}
	var dialer net.Dialer
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return Result{Healthy: false, CheckedAt: c.now(), Message: err.Error()}, nil
	}
	_ = conn.Close()
	return Result{Healthy: true, CheckedAt: c.now(), Message: "tcp connection succeeded"}, nil
}
