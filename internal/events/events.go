package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

const (
	TypeNodeRegistered        = "node.registered"
	TypeNodeHeartbeat         = "node.heartbeat"
	TypeNodeShutdown          = "node.shutdown"
	TypeNodeStatusChanged     = "node.status.changed"
	TypeServiceCreated        = "service.created"
	TypeServiceUpdated        = "service.updated"
	TypeServiceDeleted        = "service.deleted"
	TypeServiceScaled         = "service.scaled"
	TypeRolloutStarted        = "service.rollout.started"
	TypeRollbackStarted       = "service.rollback.started"
	TypeTaskAssigned          = "task.assigned"
	TypeTaskStatus            = "task.status"
	TypeTaskHealthFailed      = "task.health.failed"
	TypeTaskHealthUnhealthy   = "task.health.unhealthy"
	TypeReconcilerTaskCreated = "reconciler.task.created"
	TypeReconcilerTaskStopped = "reconciler.task.stopped"
	TypeRolloutManagerStarted = "rollout.started"
)

type Event = types.Event

type Filter struct {
	ServiceID types.ServiceID
	TaskID    types.TaskID
	NodeID    types.NodeID
	Type      string
	Severity  types.EventSeverity
	Since     time.Time
	Limit     int
}

type Store interface {
	AppendEvent(ctx context.Context, event types.Event) (types.Event, error)
}

type EmitOption func(*emitConfig)

type emitConfig struct {
	strict bool
	logger *slog.Logger
}

func Strict() EmitOption {
	return func(cfg *emitConfig) {
		cfg.strict = true
	}
}

func WithLogger(logger *slog.Logger) EmitOption {
	return func(cfg *emitConfig) {
		cfg.logger = logger
	}
}

func Emit(ctx context.Context, store Store, event types.Event, opts ...EmitOption) error {
	if store == nil {
		return nil
	}
	cfg := emitConfig{logger: slog.Default()}
	for _, opt := range opts {
		opt(&cfg)
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	if event.Severity == "" {
		event.Severity = types.EventInfo
	}
	if _, err := store.AppendEvent(ctx, event); err != nil {
		if cfg.strict {
			return err
		}
		if cfg.logger != nil {
			cfg.logger.Warn("event emission failed", "event_type", event.Type, "error", err)
		}
	}
	return nil
}
