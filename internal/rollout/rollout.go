package rollout

import (
	"context"
	"time"

	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
)

type Plan struct {
	ServiceID string
	Image     string
	BatchSize int
}

type Manager struct {
	events events.Store
	now    func() time.Time
}

func NewManager(eventStore ...events.Store) *Manager {
	var store events.Store
	if len(eventStore) > 0 {
		store = eventStore[0]
	}
	return &Manager{
		events: store,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (m *Manager) Start(ctx context.Context, plan Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = events.Emit(ctx, m.events, types.Event{
		Type:              events.TypeRolloutManagerStarted,
		Severity:          types.EventInfo,
		Source:            "rollout",
		Message:           "rollout manager started rollout",
		RelatedObjectType: "service",
		RelatedObjectID:   plan.ServiceID,
		Timestamp:         m.now(),
	})
	return nil
}
