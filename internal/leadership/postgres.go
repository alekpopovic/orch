package leadership

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresElector struct {
	pool          *pgxpool.Pool
	checkInterval time.Duration
}

func NewPostgresElector(pool *pgxpool.Pool) *PostgresElector {
	return &PostgresElector{pool: pool, checkInterval: 5 * time.Second}
}

func (e *PostgresElector) Acquire(ctx context.Context, name string) (Lease, error) {
	if e.pool == nil {
		return nil, fmt.Errorf("postgres pool is required")
	}
	conn, err := e.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire lock connection: %w", err)
	}
	key := advisoryLockKey(name)
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&acquired); err != nil {
		conn.Release()
		return nil, fmt.Errorf("try advisory lock %s: %w", name, err)
	}
	if !acquired {
		conn.Release()
		return nil, fmt.Errorf("%w: %s", ErrLockHeld, name)
	}
	lease := &postgresLease{
		name:          name,
		key:           key,
		conn:          conn,
		done:          make(chan struct{}),
		checkInterval: e.checkInterval,
	}
	go lease.monitor(ctx)
	return lease, nil
}

type postgresLease struct {
	name          string
	key           int64
	conn          *pgxpool.Conn
	done          chan struct{}
	checkInterval time.Duration
	closeOnce     sync.Once
}

func (l *postgresLease) Name() string {
	return l.name
}

func (l *postgresLease) Done() <-chan struct{} {
	return l.done
}

func (l *postgresLease) Release(ctx context.Context) error {
	select {
	case <-l.done:
		return nil
	default:
	}
	defer l.close()
	var unlocked bool
	if err := l.conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, l.key).Scan(&unlocked); err != nil {
		return fmt.Errorf("unlock advisory lock %s: %w", l.name, err)
	}
	return nil
}

func (l *postgresLease) monitor(ctx context.Context) {
	interval := l.checkInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			l.close()
			return
		case <-l.done:
			return
		case <-ticker.C:
			var ok int
			if err := l.conn.QueryRow(ctx, `SELECT 1`).Scan(&ok); err != nil {
				l.close()
				return
			}
		}
	}
}

func (l *postgresLease) close() {
	l.closeOnce.Do(func() {
		close(l.done)
		l.conn.Release()
	})
}
