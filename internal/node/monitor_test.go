package node

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestMonitorCheckOnceMarksStaleNodesOffline(t *testing.T) {
	timeout := 30 * time.Second
	marker := &fakeOfflineMarker{
		nodes: []types.Node{{ID: "node-a", Status: types.NodeOffline}},
	}
	monitor := NewMonitor(marker, slog.New(slog.NewTextHandler(io.Discard, nil)), timeout, time.Second)

	offline, err := monitor.CheckOnce(context.Background())
	if err != nil {
		t.Fatalf("check once: %v", err)
	}
	if marker.timeout != timeout {
		t.Fatalf("expected timeout %s, got %s", timeout, marker.timeout)
	}
	if len(offline) != 1 || offline[0].ID != "node-a" {
		t.Fatalf("unexpected offline nodes %#v", offline)
	}
}

func TestMonitorCheckOnceRequiresControlPlane(t *testing.T) {
	monitor := NewMonitor(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), 30*time.Second, time.Second)

	if _, err := monitor.CheckOnce(context.Background()); err == nil {
		t.Fatal("expected missing control plane error")
	}
}

func TestMonitorRunReturnsInitialCheckError(t *testing.T) {
	expected := errors.New("marker failed")
	monitor := NewMonitor(&fakeOfflineMarker{err: expected}, slog.New(slog.NewTextHandler(io.Discard, nil)), 30*time.Second, time.Second)

	if err := monitor.Run(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

type fakeOfflineMarker struct {
	timeout time.Duration
	nodes   []types.Node
	err     error
}

func (f *fakeOfflineMarker) MarkStaleNodesOffline(_ context.Context, timeout time.Duration) ([]types.Node, error) {
	f.timeout = timeout
	return f.nodes, f.err
}
