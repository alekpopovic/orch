package leadership

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"
)

var ErrLockHeld = errors.New("leader lock is already held")

type LeaderElector interface {
	Acquire(ctx context.Context, name string) (Lease, error)
}

type Lease interface {
	Name() string
	Done() <-chan struct{}
	Release(ctx context.Context) error
}

type Metrics interface {
	SetLeaderStatus(controller string, leader bool)
	IncLeaderAcquisitionFailure(controller string)
}

type NoopMetrics struct{}

func (NoopMetrics) SetLeaderStatus(string, bool)       {}
func (NoopMetrics) IncLeaderAcquisitionFailure(string) {}

func RunWithLeadership(ctx context.Context, elector LeaderElector, controller string, logger *slog.Logger, metrics Metrics, run func(context.Context) error) error {
	if elector == nil {
		return fmt.Errorf("leader elector is required")
	}
	if run == nil {
		return fmt.Errorf("leader callback is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = NoopMetrics{}
	}
	lease, err := elector.Acquire(ctx, controller)
	if err != nil {
		metrics.IncLeaderAcquisitionFailure(controller)
		return err
	}
	metrics.SetLeaderStatus(controller, true)
	defer metrics.SetLeaderStatus(controller, false)
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := lease.Release(releaseCtx); err != nil {
			logger.Warn("release leader lease failed", "controller", controller, "error", err)
		}
	}()

	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-lease.Done():
			cancel()
		}
	}()
	return run(leaderCtx)
}

func advisoryLockKey(name string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte("orch:" + name))
	return int64(hash.Sum64())
}

type LocalElector struct {
	mu     sync.Mutex
	leases map[string]*localLease
}

func NewLocalElector() *LocalElector {
	return &LocalElector{leases: map[string]*localLease{}}
}

func (e *LocalElector) Acquire(ctx context.Context, name string) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if current := e.leases[name]; current != nil && !current.released {
		return nil, fmt.Errorf("%w: %s", ErrLockHeld, name)
	}
	lease := &localLease{name: name, elector: e, done: make(chan struct{})}
	e.leases[name] = lease
	return lease, nil
}

func (e *LocalElector) Lose(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if lease := e.leases[name]; lease != nil {
		lease.closeLocked()
	}
}

type localLease struct {
	name     string
	elector  *LocalElector
	done     chan struct{}
	released bool
}

func (l *localLease) Name() string {
	return l.name
}

func (l *localLease) Done() <-chan struct{} {
	return l.done
}

func (l *localLease) Release(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.elector.mu.Lock()
	defer l.elector.mu.Unlock()
	l.closeLocked()
	if l.elector.leases[l.name] == l {
		delete(l.elector.leases, l.name)
	}
	return nil
}

func (l *localLease) closeLocked() {
	if l.released {
		return
	}
	l.released = true
	close(l.done)
}
