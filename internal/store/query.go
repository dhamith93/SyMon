package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/jackc/pgx/v5"
)

// HostSummary is one row of the fleet overview, built from each host's
// latest snapshot
type HostSummary struct {
	Name     string
	LastSeen time.Time
	// Time of the latest snapshot, zero if the host never sent one
	Time          time.Time
	OS            string
	UptimeSeconds float64
	CPUPct        float64
	MemUsedPct    float64
	SwapUsedPct   float64
	// DiskUsedPct is the fullest disk
	DiskUsedPct  float64
	RxBps        float64
	TxBps        float64
	ActiveAlerts int
}

func (s *Store) FleetSummary(ctx context.Context) ([]HostSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT h.name, h.last_seen, l.time, l.snapshot,
		       (SELECT count(*) FROM alerts a WHERE a.host_id = h.id AND a.resolved_at IS NULL)
		FROM hosts h
		LEFT JOIN host_latest l ON l.host_id = h.id
		ORDER BY h.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := []HostSummary{}
	for rows.Next() {
		var summary HostSummary
		var lastSeen, snapshotTime *time.Time
		var snapshot []byte
		if err := rows.Scan(&summary.Name, &lastSeen, &snapshotTime, &snapshot, &summary.ActiveAlerts); err != nil {
			return nil, err
		}
		if lastSeen != nil {
			summary.LastSeen = *lastSeen
		}
		if snapshotTime != nil {
			summary.Time = *snapshotTime
			var data monitor.MonitorData
			if err := json.Unmarshal(snapshot, &data); err != nil {
				return nil, err
			}
			fillSummary(&summary, &data)
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func fillSummary(summary *HostSummary, data *monitor.MonitorData) {
	summary.OS = data.System.OS
	summary.UptimeSeconds = data.System.UpTimeSeconds
	summary.CPUPct = float64(data.ProcUsage.LoadAvg)
	summary.MemUsedPct = data.Memory.PercentageUsed
	summary.SwapUsedPct = data.Swap.PercentageUsed
	for _, disk := range data.Disk {
		if pct := parsePercent(disk.Usage.Usage); pct != nil && *pct > summary.DiskUsedPct {
			summary.DiskUsedPct = *pct
		}
	}
	for _, network := range data.Networks {
		if network.Rates != nil {
			summary.RxBps += network.Rates.RxBytesPerSec
			summary.TxBps += network.Rates.TxBytesPerSec
		}
	}
}

// LatestSnapshot returns a host's newest snapshot as the agent sent it
func (s *Store) LatestSnapshot(ctx context.Context, host string) (time.Time, []byte, error) {
	var at time.Time
	var snapshot []byte
	err := s.pool.QueryRow(ctx, `
		SELECT l.time, l.snapshot FROM host_latest l
		JOIN hosts h ON h.id = l.host_id
		WHERE h.name = $1`, host).Scan(&at, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil, ErrNotFound
	}
	return at, snapshot, err
}

// Processes returns the top process lists from the newest snapshot at or
// before the given time
func (s *Store) Processes(ctx context.Context, host string, at time.Time) (time.Time, []byte, error) {
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return time.Time{}, nil, err
	}
	var snapshotTime time.Time
	var processes []byte
	err = s.pool.QueryRow(ctx, `
		SELECT time, processes FROM process_snapshots
		WHERE host_id = $1 AND time <= $2
		ORDER BY time DESC LIMIT 1`, hostID, at).Scan(&snapshotTime, &processes)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil, ErrNotFound
	}
	return snapshotTime, processes, err
}

// CustomMetricNames lists the custom metrics a host has sent within the
// hourly retention
func (s *Store) CustomMetricNames(ctx context.Context, host string) ([]string, error) {
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT DISTINCT name FROM custom_metrics_1h WHERE host_id = $1 ORDER BY name", hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
