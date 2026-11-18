package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/proctrace"
	"github.com/jackc/pgx/v5"
)

// bucketColumns are disk_latency_buckets' columns b0 to b18, in order
var bucketColumns = func() string {
	columns := make([]string, proctrace.LatencyBuckets)
	for i := range columns {
		columns[i] = fmt.Sprintf("b%d", i)
	}
	return strings.Join(columns, ", ")
}()

// how many steps DiskLatency splits a range into at most, about one per
// pixel column of the heatmap
const latencySteps = 300

// LatencyStep is one step of a disk's latency histogram. Buckets has
// proctrace.LatencyBuckets counts. The quantiles are in ms, and zero for a
// step without IOs.
type LatencyStep struct {
	Time    time.Time
	Buckets []int64
	P50     float64
	P95     float64
	P99     float64
}

type DiskLatencyResult struct {
	// the disks with a histogram in the range, the most IOs first
	Devices []string
	// the disk and op the steps are for, empty when there are no disks
	Device string
	Op     string
	Step   time.Duration
	Steps  []LatencyStep
}

// DiskLatency returns a disk's latency histogram over a range, from the
// eBPF tracer's buckets. op is "read" or "write". An empty device, or one
// without a histogram in the range, picks the busiest disk.
func (s *Store) DiskLatency(ctx context.Context, host string, device string, op string, from time.Time, to time.Time) (DiskLatencyResult, error) {
	if !to.After(from) {
		return DiskLatencyResult{}, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	if op != "read" && op != "write" {
		return DiskLatencyResult{}, fmt.Errorf("%w: op must be read or write", ErrInvalid)
	}
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return DiskLatencyResult{}, err
	}

	source, step := pickSource(seriesDef{table: "disk_latency_buckets"}, from, to, time.Now(), latencySteps, s.retention)
	table, timeColumn := "disk_latency_buckets", "time"
	if source != "raw" {
		table, timeColumn = "disk_latency_buckets_"+source, "bucket"
	}
	result := DiskLatencyResult{Devices: []string{}, Op: op, Step: step, Steps: []LatencyStep{}}

	// table and column names are constants, never from the caller
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT device FROM %[1]s
		WHERE host_id = $1 AND %[2]s >= $2 AND %[2]s < $3
		GROUP BY device
		ORDER BY sum(%[3]s) DESC, device`, table, timeColumn, strings.ReplaceAll(bucketColumns, ", ", " + ")),
		hostID, from, to)
	if err != nil {
		return DiskLatencyResult{}, err
	}
	result.Devices, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return DiskLatencyResult{}, err
	}
	if len(result.Devices) == 0 {
		result.Devices = []string{}
		return result, nil
	}
	result.Device = device
	if !slices.Contains(result.Devices, device) {
		result.Device = result.Devices[0]
	}

	sums := make([]string, proctrace.LatencyBuckets)
	for i := range sums {
		sums[i] = fmt.Sprintf("sum(b%d)::bigint", i)
	}
	rows, err = s.pool.Query(ctx, fmt.Sprintf(`
		SELECT time_bucket($1::interval, %[2]s) AS t, %[3]s
		FROM %[1]s
		WHERE host_id = $2 AND device = $3 AND op = $4 AND %[2]s >= $5 AND %[2]s < $6
		GROUP BY t
		ORDER BY t`, table, timeColumn, strings.Join(sums, ", ")),
		fmt.Sprintf("%d seconds", int64(step.Seconds())), hostID, result.Device, op, from, to)
	if err != nil {
		return DiskLatencyResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		latency := LatencyStep{Buckets: make([]int64, proctrace.LatencyBuckets)}
		dest := []any{&latency.Time}
		for i := range latency.Buckets {
			dest = append(dest, &latency.Buckets[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return DiskLatencyResult{}, err
		}
		latency.P50, _ = proctrace.Quantile(latency.Buckets, 0.5)
		latency.P95, _ = proctrace.Quantile(latency.Buckets, 0.95)
		latency.P99, _ = proctrace.Quantile(latency.Buckets, 0.99)
		result.Steps = append(result.Steps, latency)
	}
	return result, rows.Err()
}
