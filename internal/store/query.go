package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// WorstSeverity is the highest severity of the open alerts, 0 if none
	WorstSeverity int
	// Containers is how many containers were running at the latest snapshot
	Containers int
}

func (s *Store) FleetSummary(ctx context.Context) ([]HostSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT h.name, h.last_seen, l.time, l.snapshot, count(a.id), coalesce(max(a.severity), 0)
		FROM hosts h
		LEFT JOIN host_latest l ON l.host_id = h.id
		LEFT JOIN alerts a ON a.host_id = h.id AND a.resolved_at IS NULL
		GROUP BY h.id, l.host_id
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
		if err := rows.Scan(&summary.Name, &lastSeen, &snapshotTime, &snapshot, &summary.ActiveAlerts, &summary.WorstSeverity); err != nil {
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
	summary.Containers = len(data.Containers)
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

type LatestSnapshot struct {
	Time time.Time
	// Snapshot is the MonitorData as the agent sent it
	Snapshot []byte
	LastSeen time.Time
}

// LatestSnapshot returns a host's newest snapshot and when it was last heard from
func (s *Store) LatestSnapshot(ctx context.Context, host string) (LatestSnapshot, error) {
	var latest LatestSnapshot
	var lastSeen *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT l.time, l.snapshot, h.last_seen FROM host_latest l
		JOIN hosts h ON h.id = l.host_id
		WHERE h.name = $1`, host).Scan(&latest.Time, &latest.Snapshot, &lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return LatestSnapshot{}, ErrNotFound
	}
	if lastSeen != nil {
		latest.LastSeen = *lastSeen
	}
	return latest, err
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

// ProcessUsage is one program's share of a time range. Processes with the
// same name, like the workers of a web server, are added up per snapshot.
type ProcessUsage struct {
	Name    string
	CPUAvg  float64
	CPUPeak float64
	MemAvg  float64
	MemPeak float64
	// SeenPct is the share of snapshots the program was in the top lists
	SeenPct float64
}

type ProcessUsageResult struct {
	Snapshots int
	// FirstTime is the first snapshot in the range, zero when there is none
	FirstTime time.Time
	// Processes has the busiest programs by CPU and by memory, busiest CPU first
	Processes []ProcessUsage
}

// processUsageLimit is how many programs ProcessUsage returns for each of
// CPU and memory
const processUsageLimit = 15

// ProcessUsage adds up each program's CPU and memory over a range. Snapshots
// only keep the top processes, so a program counts as 0 where it was not
// among them, and the averages are a lower bound.
func (s *Store) ProcessUsage(ctx context.Context, host string, from time.Time, to time.Time) (ProcessUsageResult, error) {
	if !to.After(from) {
		return ProcessUsageResult{}, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return ProcessUsageResult{}, err
	}

	result := ProcessUsageResult{Processes: []ProcessUsage{}}
	var first *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT count(*), min(time) FROM process_snapshots
		WHERE host_id = $1 AND time >= $2 AND time < $3`, hostID, from, to).Scan(&result.Snapshots, &first)
	if err != nil || result.Snapshots == 0 {
		return result, err
	}
	result.FirstTime = *first

	// the CPU and Memory lists overlap, so each pid counts once per snapshot
	rows, err := s.pool.Query(ctx, `
		WITH processes AS (
			SELECT DISTINCT ON (s.time, p->>'Pid')
			       s.time,
			       coalesce(nullif(p->>'Name', ''), nullif(p->>'ExecPath', ''), 'unknown') AS name,
			       coalesce((p->>'CPUUsage')::float8, 0) AS cpu,
			       coalesce((p->>'MemUsage')::float8, 0) AS mem
			FROM process_snapshots s,
			     jsonb_path_query(s.processes, '$.*[*] ? (@.type() == "object")') AS p
			WHERE s.host_id = $1 AND s.time >= $2 AND s.time < $3
		),
		per_snapshot AS (
			SELECT time, name, sum(cpu) AS cpu, sum(mem) AS mem
			FROM processes
			GROUP BY time, name
		),
		ranked AS (
			SELECT name, sum(cpu) AS cpu_sum, max(cpu) AS cpu_peak, sum(mem) AS mem_sum, max(mem) AS mem_peak, count(*) AS seen,
			       row_number() OVER (ORDER BY sum(cpu) DESC, name) AS cpu_rank,
			       row_number() OVER (ORDER BY sum(mem) DESC, name) AS mem_rank
			FROM per_snapshot
			GROUP BY name
		)
		SELECT name, cpu_sum, cpu_peak, mem_sum, mem_peak, seen FROM ranked
		WHERE cpu_rank <= $4 OR mem_rank <= $4
		ORDER BY cpu_sum DESC, name`, hostID, from, to, processUsageLimit)
	if err != nil {
		return ProcessUsageResult{}, err
	}
	defer rows.Close()

	snapshots := float64(result.Snapshots)
	for rows.Next() {
		var usage ProcessUsage
		var cpuSum, memSum float64
		var seen int
		if err := rows.Scan(&usage.Name, &cpuSum, &usage.CPUPeak, &memSum, &usage.MemPeak, &seen); err != nil {
			return ProcessUsageResult{}, err
		}
		usage.CPUAvg = cpuSum / snapshots
		usage.MemAvg = memSum / snapshots
		usage.SeenPct = 100 * float64(seen) / snapshots
		result.Processes = append(result.Processes, usage)
	}
	return result, rows.Err()
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
