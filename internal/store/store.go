// Package store keeps SyMon's data in TimescaleDB: registered hosts, their
// metrics as hypertables with 1 minute and 1 hour rollups, and alerts.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound is returned when a host or a value does not exist
var ErrNotFound = errors.New("not found")

// ErrInvalid is returned for bad input, like an unknown metric or a
// custom metric value that is not a number
var ErrInvalid = errors.New("invalid request")

// Retention is how long each resolution of metric data is kept
type Retention struct {
	Raw    time.Duration
	Minute time.Duration
	Hour   time.Duration
}

// DefaultRetention keeps raw data for a week, 1 minute rollups for a month
// and 1 hour rollups for a year
var DefaultRetention = Retention{
	Raw:    7 * 24 * time.Hour,
	Minute: 30 * 24 * time.Hour,
	Hour:   365 * 24 * time.Hour,
}

type Store struct {
	pool      *pgxpool.Pool
	retention Retention
}

// Open connects to the database at url, a postgres:// connection string
func Open(ctx context.Context, url string, retention Retention) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("cannot open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("cannot reach database: %w", err)
	}
	return &Store{pool: pool, retention: retention}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

// Migrate applies the embedded migrations that have not run yet, each in
// its own transaction
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return fmt.Errorf("cannot create schema_migrations: %w", err)
	}

	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, file := range files {
		if err := s.applyMigration(ctx, file); err != nil {
			return fmt.Errorf("migration %s: %w", file, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, file string) error {
	var applied bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", file).Scan(&applied)
	if err != nil || applied {
		return err
	}

	sql, err := migrations.ReadFile(file)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// no arguments, so pgx sends this with the simple protocol, which
	// allows many statements in one call
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", file); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var (
	rawTables    = []string{"host_metrics", "cpu_core_metrics", "disk_metrics", "disk_io_metrics", "net_metrics", "temp_metrics", "custom_metrics", "service_status", "process_snapshots", "container_metrics"}
	minuteTables = []string{"host_metrics_1m", "disk_metrics_1m", "disk_io_metrics_1m", "net_metrics_1m", "temp_metrics_1m", "custom_metrics_1m", "container_metrics_1m"}
	hourTables   = []string{"host_metrics_1h", "disk_metrics_1h", "disk_io_metrics_1h", "net_metrics_1h", "temp_metrics_1h", "custom_metrics_1h", "container_metrics_1h"}
)

// ApplyRetention replaces the retention policies with the configured ones,
// so a changed setting takes effect on the next collector start
func (s *Store) ApplyRetention(ctx context.Context) error {
	groups := []struct {
		tables []string
		keep   time.Duration
	}{
		{rawTables, s.retention.Raw},
		{minuteTables, s.retention.Minute},
		{hourTables, s.retention.Hour},
	}
	for _, group := range groups {
		for _, table := range group.tables {
			if _, err := s.pool.Exec(ctx, "SELECT remove_retention_policy($1, if_exists => true)", table); err != nil {
				return fmt.Errorf("cannot remove retention on %s: %w", table, err)
			}
			keep := fmt.Sprintf("%d seconds", int64(group.keep.Seconds()))
			if _, err := s.pool.Exec(ctx, "SELECT add_retention_policy($1, drop_after => $2::interval)", table, keep); err != nil {
				return fmt.Errorf("cannot add retention on %s: %w", table, err)
			}
		}
	}
	return nil
}

// PurgeResolvedAlerts deletes alerts resolved longer ago than the longest
// retention, since they are not dropped by timescale policies
func (s *Store) PurgeResolvedAlerts(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, "DELETE FROM alerts WHERE resolved_at < $1", time.Now().Add(-s.retention.Hour))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
