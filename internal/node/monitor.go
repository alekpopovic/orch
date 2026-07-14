package node

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

type OfflineMarker interface {
	MarkStaleNodesOffline(ctx context.Context, timeout time.Duration) ([]types.Node, error)
}

type Monitor struct {
	controlPlane OfflineMarker
	logger       *slog.Logger
	timeout      time.Duration
	interval     time.Duration
}

func NewMonitor(controlPlane OfflineMarker, logger *slog.Logger, timeout time.Duration, interval time.Duration) *Monitor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Monitor{
		controlPlane: controlPlane,
		logger:       logger,
		timeout:      timeout,
		interval:     interval,
	}
}

func (m *Monitor) Run(ctx context.Context) error {
	if _, err := m.CheckOnce(ctx); err != nil {
		return err
	}
	if m.interval <= 0 {
		return fmt.Errorf("node monitor interval must be positive")
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := m.CheckOnce(ctx); err != nil {
				m.logger.Warn("node monitor check failed", "error", err)
			}
		}
	}
}

func (m *Monitor) CheckOnce(ctx context.Context) ([]types.Node, error) {
	if m.controlPlane == nil {
		return nil, fmt.Errorf("node monitor control plane is required")
	}
	if m.timeout <= 0 {
		return nil, fmt.Errorf("node heartbeat timeout must be positive")
	}
	offline, err := m.controlPlane.MarkStaleNodesOffline(ctx, m.timeout)
	if err != nil {
		return nil, err
	}
	for _, node := range offline {
		m.logger.Warn("node marked offline after stale heartbeat", "node_id", node.ID, "last_heartbeat_at", node.LastHeartbeatAt)
	}
	return offline, nil
}
