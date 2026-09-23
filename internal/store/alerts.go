package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/jackc/pgx/v5"
)

type Alert struct {
	ID     int64
	Host   string
	Rule   string
	Metric string
	// Target is the disk, service or custom metric name, empty otherwise
	Target string
	// Severity is 1 for warning and 2 for critical
	Severity  int
	Value     float64
	StartedAt time.Time
	UpdatedAt time.Time
	// ResolvedAt is nil while the alert is open
	ResolvedAt *time.Time
}

// LatestValue returns the newest value of the metric an alert rule watches.
// metric is the rule's MetricName. Services report 1 when running and 0
// when not.
func (s *Store) LatestValue(ctx context.Context, host string, metric string, target string, isCustom bool) (float64, time.Time, error) {
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return 0, time.Time{}, err
	}

	var sql string
	args := []any{hostID}
	switch {
	case isCustom:
		sql = "SELECT time, value FROM custom_metrics WHERE host_id = $1 AND name = $2 ORDER BY time DESC LIMIT 1"
		args = append(args, metric)
	case metric == monitor.PROC_USAGE:
		sql = "SELECT time, cpu_pct FROM host_metrics WHERE host_id = $1 ORDER BY time DESC LIMIT 1"
	case metric == monitor.MEMORY:
		sql = "SELECT time, mem_used_pct FROM host_metrics WHERE host_id = $1 ORDER BY time DESC LIMIT 1"
	case metric == monitor.SWAP:
		sql = "SELECT time, swap_used_pct FROM host_metrics WHERE host_id = $1 ORDER BY time DESC LIMIT 1"
	case metric == monitor.DISKS:
		sql = "SELECT time, used_pct FROM disk_metrics WHERE host_id = $1 AND device = $2 ORDER BY time DESC LIMIT 1"
		args = append(args, target)
	case metric == monitor.SERVICES:
		sql = "SELECT time, CASE WHEN running THEN 1.0 ELSE 0.0 END FROM service_status WHERE host_id = $1 AND name = $2 ORDER BY time DESC LIMIT 1"
		args = append(args, target)
	default:
		return 0, time.Time{}, fmt.Errorf("%w: metric %q cannot be alerted on", ErrInvalid, metric)
	}

	var at time.Time
	var value *float64
	err = s.pool.QueryRow(ctx, sql, args...).Scan(&at, &value)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && value == nil) {
		return 0, time.Time{}, ErrNotFound
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	return *value, at, nil
}

const alertColumns = "a.id, h.name, a.rule, a.metric, a.target, a.severity, a.value, a.started_at, a.updated_at, a.resolved_at"

func scanAlert(row pgx.Row) (Alert, error) {
	var alert Alert
	var value *float64
	err := row.Scan(&alert.ID, &alert.Host, &alert.Rule, &alert.Metric, &alert.Target, &alert.Severity,
		&value, &alert.StartedAt, &alert.UpdatedAt, &alert.ResolvedAt)
	if value != nil {
		alert.Value = *value
	}
	return alert, err
}

// OpenAlert returns the unresolved alert for a host, rule and target, or
// nil if there is none
func (s *Store) OpenAlert(ctx context.Context, host string, rule string, metric string, target string) (*Alert, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+alertColumns+`
		FROM alerts a JOIN hosts h ON h.id = a.host_id
		WHERE h.name = $1 AND a.rule = $2 AND a.metric = $3 AND a.target = $4 AND a.resolved_at IS NULL`,
		host, rule, metric, target)
	alert, err := scanAlert(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

// CreateAlert opens an alert and returns its id
func (s *Store) CreateAlert(ctx context.Context, alert Alert) (int64, error) {
	hostID, err := s.hostID(ctx, alert.Host)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO alerts (host_id, rule, metric, target, severity, value, started_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING id`,
		hostID, alert.Rule, alert.Metric, alert.Target, alert.Severity, alert.Value, alert.StartedAt).Scan(&id)
	return id, err
}

// UpdateAlert changes an open alert's severity, for example from warning
// to critical
func (s *Store) UpdateAlert(ctx context.Context, id int64, severity int, value float64, at time.Time) error {
	_, err := s.pool.Exec(ctx, "UPDATE alerts SET severity = $2, value = $3, updated_at = $4 WHERE id = $1",
		id, severity, value, at)
	return err
}

func (s *Store) ResolveAlert(ctx context.Context, id int64, value float64, at time.Time) error {
	_, err := s.pool.Exec(ctx, "UPDATE alerts SET value = $2, updated_at = $3, resolved_at = $3 WHERE id = $1",
		id, value, at)
	return err
}

type AlertFilter struct {
	// Host limits the list to one host, empty means all
	Host     string
	OpenOnly bool
	// From and To limit by start time, zero means no limit
	From time.Time
	To   time.Time
}

// Alerts lists alerts, newest first
func (s *Store) Alerts(ctx context.Context, filter AlertFilter) ([]Alert, error) {
	sql := `SELECT ` + alertColumns + ` FROM alerts a JOIN hosts h ON h.id = a.host_id WHERE true`
	args := []any{}
	if filter.Host != "" {
		args = append(args, filter.Host)
		sql += fmt.Sprintf(" AND h.name = $%d", len(args))
	}
	if filter.OpenOnly {
		sql += " AND a.resolved_at IS NULL"
	}
	if !filter.From.IsZero() {
		args = append(args, filter.From)
		sql += fmt.Sprintf(" AND a.started_at >= $%d", len(args))
	}
	if !filter.To.IsZero() {
		args = append(args, filter.To)
		sql += fmt.Sprintf(" AND a.started_at < $%d", len(args))
	}
	sql += " ORDER BY a.started_at DESC"

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := []Alert{}
	for rows.Next() {
		alert, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}
