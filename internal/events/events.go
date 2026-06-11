package events

import (
	"context"
	"time"
)

type Event struct {
	Type       string
	ResourceID string
	Message    string
	OccurredAt time.Time
}

type Sink interface {
	Publish(ctx context.Context, event Event) error
}
