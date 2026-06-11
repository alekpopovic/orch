package rollout

import "context"

type Plan struct {
	ServiceID string
	Image     string
	BatchSize int
}

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Start(ctx context.Context, plan Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
