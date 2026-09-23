package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrHostExists is returned when registering a name that is already taken
var ErrHostExists = errors.New("host already registered")

type Host struct {
	ID       int64
	Name     string
	Timezone string
	// LastSeen is zero until the host pings or sends data
	LastSeen time.Time
}

func (s *Store) AddHost(ctx context.Context, name string, timezone string) error {
	tag, err := s.pool.Exec(ctx, "INSERT INTO hosts (name, timezone) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING", name, timezone)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrHostExists
	}
	return nil
}

// RemoveHost unregisters a host. Its metrics stay until retention drops them.
func (s *Store) RemoveHost(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM hosts WHERE name = $1", name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Hosts(ctx context.Context) ([]Host, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name, timezone, last_seen FROM hosts ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hosts := []Host{}
	for rows.Next() {
		var host Host
		var lastSeen *time.Time
		if err := rows.Scan(&host.ID, &host.Name, &host.Timezone, &lastSeen); err != nil {
			return nil, err
		}
		if lastSeen != nil {
			host.LastSeen = *lastSeen
		}
		hosts = append(hosts, host)
	}
	return hosts, rows.Err()
}

func (s *Store) hostID(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, "SELECT id FROM hosts WHERE name = $1", name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// Heartbeat records that a host was heard from at the given time
func (s *Store) Heartbeat(ctx context.Context, name string, at time.Time) error {
	tag, err := s.pool.Exec(ctx, "UPDATE hosts SET last_seen = greatest(last_seen, $2) WHERE name = $1", name, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LastSeen returns when a host last pinged or sent data, zero if never
func (s *Store) LastSeen(ctx context.Context, name string) (time.Time, error) {
	var lastSeen *time.Time
	err := s.pool.QueryRow(ctx, "SELECT last_seen FROM hosts WHERE name = $1", name).Scan(&lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil || lastSeen == nil {
		return time.Time{}, err
	}
	return *lastSeen, nil
}
