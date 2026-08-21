package migrations

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const AdvisoryLockID int64 = 675784603013

type Status struct {
	Current string   `json:"current"`
	Latest  string   `json:"latest"`
	Pending []string `json:"pending"`
}
type Migration struct {
	Version  string
	UpPath   string
	DownPath string
}

func Discover(directory string) ([]Migration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	byVersion := map[string]*Migration{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			continue
		}
		version := parts[0]
		item := byVersion[version]
		if item == nil {
			item = &Migration{Version: version}
			byVersion[version] = item
		}
		path := filepath.Join(directory, name)
		if strings.HasSuffix(name, ".up.sql") {
			item.UpPath = path
		}
		if strings.HasSuffix(name, ".down.sql") {
			item.DownPath = path
		}
	}
	out := make([]Migration, 0, len(byVersion))
	for _, v := range byVersion {
		if v.UpPath != "" {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

type Locker interface {
	Lock(context.Context) error
	Unlock(context.Context) error
}

func WithLock(ctx context.Context, locker Locker, fn func() error) (resultErr error) {
	if err := locker.Lock(ctx); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		if err := locker.Unlock(context.Background()); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("release migration lock: %w", err)
		}
	}()
	return fn()
}

type Runner struct {
	pool      *pgxpool.Pool
	directory string
}

func Open(ctx context.Context, databaseURL, directory string) (*Runner, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Runner{pool: pool, directory: directory}, nil
}
func (r *Runner) Close() {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
}
func (r *Runner) Status(ctx context.Context) (Status, error) {
	migrations, err := Discover(r.directory)
	if err != nil {
		return Status{}, err
	}
	if _, err = r.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc',now()))`); err != nil {
		return Status{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return Status{}, err
	}
	defer rows.Close()
	applied := map[string]struct{}{}
	current := ""
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return Status{}, err
		}
		applied[v] = struct{}{}
		current = v
	}
	status := Status{Current: current}
	if len(migrations) > 0 {
		status.Latest = migrations[len(migrations)-1].Version
	}
	for _, m := range migrations {
		if _, ok := applied[m.Version]; !ok {
			status.Pending = append(status.Pending, m.Version)
		}
	}
	return status, rows.Err()
}
func (r *Runner) Up(ctx context.Context) error {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return WithLock(ctx, connLocker{conn}, func() error {
		if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc',now()))`); err != nil {
			return err
		}
		migrations, err := Discover(r.directory)
		if err != nil {
			return err
		}
		for _, m := range migrations {
			var exists bool
			if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, m.Version).Scan(&exists); err != nil {
				return err
			}
			if exists {
				continue
			}
			data, err := os.ReadFile(m.UpPath)
			if err != nil {
				return err
			}
			tx, err := conn.Begin(ctx)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, string(data)); err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, m.Version)
			}
			if err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("apply migration %s: %w", m.Version, err)
			}
			if err = tx.Commit(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *Runner) Down(ctx context.Context, allowed bool) error {
	if !allowed {
		return fmt.Errorf("down migrations require --allow-down")
	}
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return WithLock(ctx, connLocker{conn}, func() error {
		var version string
		if err := conn.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&version); err != nil {
			return err
		}
		items, err := Discover(r.directory)
		if err != nil {
			return err
		}
		for _, m := range items {
			if m.Version != version {
				continue
			}
			if m.DownPath == "" {
				return fmt.Errorf("migration %s has no safe down path", version)
			}
			data, err := os.ReadFile(m.DownPath)
			if err != nil {
				return err
			}
			tx, err := conn.Begin(ctx)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, string(data)); err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version=$1`, version)
			}
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			return tx.Commit(ctx)
		}
		return pgx.ErrNoRows
	})
}

type connLocker struct{ conn *pgxpool.Conn }

func (l connLocker) Lock(ctx context.Context) error {
	_, err := l.conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, AdvisoryLockID)
	return err
}
func (l connLocker) Unlock(ctx context.Context) error {
	_, err := l.conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, AdvisoryLockID)
	return err
}
