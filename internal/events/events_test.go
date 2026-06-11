package events

import (
	"context"
	"errors"
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestEmitBestEffortDoesNotReturnError(t *testing.T) {
	store := &failingStore{}
	err := Emit(context.Background(), store, types.Event{Type: TypeTaskStatus})
	if err != nil {
		t.Fatalf("expected best-effort emit to ignore error, got %v", err)
	}
}

func TestEmitStrictReturnsError(t *testing.T) {
	store := &failingStore{}
	err := Emit(context.Background(), store, types.Event{Type: TypeTaskStatus}, Strict())
	if !errors.Is(err, errEmitFailed) {
		t.Fatalf("expected strict emit error, got %v", err)
	}
}

func TestEmitDefaults(t *testing.T) {
	store := &capturingStore{}
	if err := Emit(context.Background(), store, types.Event{Type: TypeTaskStatus}); err != nil {
		t.Fatalf("emit event: %v", err)
	}
	if store.event.Severity != types.EventInfo {
		t.Fatalf("expected default severity info, got %q", store.event.Severity)
	}
	if store.event.Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}

var errEmitFailed = errors.New("emit failed")

type failingStore struct{}

func (*failingStore) AppendEvent(context.Context, types.Event) (types.Event, error) {
	return types.Event{}, errEmitFailed
}

type capturingStore struct {
	event types.Event
}

func (s *capturingStore) AppendEvent(_ context.Context, event types.Event) (types.Event, error) {
	s.event = event
	return event, nil
}
