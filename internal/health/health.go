package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type ProbeType string

const (
	NoneProbe ProbeType = "none"
	HTTPProbe ProbeType = "http"
	TCPProbe  ProbeType = "tcp"
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
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Healthy: false, CheckedAt: c.now(), Message: err.Error()}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return Result{Healthy: false, CheckedAt: c.now(), Message: resp.Status}, nil
	}
	return Result{Healthy: true, CheckedAt: c.now(), Message: resp.Status}, nil
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
