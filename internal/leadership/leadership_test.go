package leadership

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestLocalElectorAllowsOneLeader(t *testing.T) {
	elector := NewLocalElector()
	first, err := elector.Acquire(context.Background(), "scheduler")
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	if _, err := elector.Acquire(context.Background(), "scheduler"); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected second acquire to fail with ErrLockHeld, got %v", err)
	}
	if err := first.Release(context.Background()); err != nil {
		t.Fatalf("release first lease: %v", err)
	}
	second, err := elector.Acquire(context.Background(), "scheduler")
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := second.Release(context.Background()); err != nil {
		t.Fatalf("release second lease: %v", err)
	}
}

func TestRunWithLeadershipStopsWhenLeaseLost(t *testing.T) {
	elector := NewLocalElector()
	metrics := &recordingLeadershipMetrics{}
	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- RunWithLeadership(context.Background(), elector, "reconciler", slog.Default(), metrics, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("controller did not start")
	}
	elector.Lose("reconciler")
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled after lock loss, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("controller did not stop after lock loss")
	}
	if metrics.status["reconciler"] {
		t.Fatalf("expected leader status false after stop")
	}
}

func TestRunWithLeadershipRecordsAcquisitionFailure(t *testing.T) {
	elector := NewLocalElector()
	lease, err := elector.Acquire(context.Background(), "rollout")
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	t.Cleanup(func() {
		if err := lease.Release(context.Background()); err != nil {
			t.Errorf("release lease: %v", err)
		}
	})
	metrics := &recordingLeadershipMetrics{}

	err = RunWithLeadership(context.Background(), elector, "rollout", slog.Default(), metrics, func(context.Context) error {
		t.Fatal("controller should not run without leadership")
		return nil
	})
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected lock held error, got %v", err)
	}
	if metrics.failures["rollout"] != 1 {
		t.Fatalf("expected acquisition failure metric, got %#v", metrics.failures)
	}
}

func TestAdvisoryLockKeyIsStable(t *testing.T) {
	if advisoryLockKey("scheduler") == 0 {
		t.Fatalf("expected non-zero advisory key")
	}
	if advisoryLockKey("scheduler") == advisoryLockKey("reconciler") {
		t.Fatalf("expected different controllers to use different keys")
	}
}

type recordingLeadershipMetrics struct {
	status   map[string]bool
	failures map[string]int
}

func (m *recordingLeadershipMetrics) SetLeaderStatus(controller string, leader bool) {
	if m.status == nil {
		m.status = map[string]bool{}
	}
	m.status[controller] = leader
}

func (m *recordingLeadershipMetrics) IncLeaderAcquisitionFailure(controller string) {
	if m.failures == nil {
		m.failures = map[string]int{}
	}
	m.failures[controller]++
}
