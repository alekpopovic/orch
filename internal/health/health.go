package health

import (
	"context"
	"time"
)

type ProbeType string

const (
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
