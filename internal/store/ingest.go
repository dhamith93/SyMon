package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/jackc/pgx/v5"
)

// SaveSnapshot stores one agent snapshot in a single transaction. The host
// must be registered.
func (s *Store) SaveSnapshot(ctx context.Context, data *monitor.MonitorData) error {
	at, err := parseUnixTime(data.UnixTime)
	if err != nil {
		return err
	}
	hostID, err := s.hostID(ctx, data.ServerId)
	if err != nil {
		return err
	}
	snapshot, err := json.Marshal(data)
	if err != nil {
		return err
	}
	processes, err := json.Marshal(data.Processes)
	if err != nil {
		return err
	}

	batch := &pgx.Batch{}
	queueHostMetrics(batch, at, hostID, data)

	for core, pct := range data.ProcUsage.CoreAvg {
		batch.Queue("INSERT INTO cpu_core_metrics (time, host_id, core, pct) VALUES ($1, $2, $3, $4)", at, hostID, core, float64(pct))
	}
	for _, disk := range data.Disk {
		batch.Queue(`INSERT INTO disk_metrics (time, host_id, device, mount, fstype, size_bytes, used_bytes, used_pct, inodes_used_pct)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			at, hostID, disk.FileSystem, disk.MountedOn, disk.Type, float64(disk.Usage.Size), float64(disk.Usage.Used),
			parsePercent(disk.Usage.Usage), parsePercent(disk.Inodes.Usage))
	}
	for _, io := range data.DiskIO {
		batch.Queue(`INSERT INTO disk_io_metrics (time, host_id, device, read_bps, write_bps, reads_ps, writes_ps, util_pct)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			at, hostID, "/dev/"+io.Device, io.ReadBytesPerSec, io.WriteBytesPerSec, io.ReadsPerSec, io.WritesPerSec, io.UtilPercent)
	}
	for _, network := range data.Networks {
		var rxBps, txBps, rxPps, txPps *float64
		if network.Rates != nil {
			rxBps, txBps = &network.Rates.RxBytesPerSec, &network.Rates.TxBytesPerSec
			rxPps, txPps = &network.Rates.RxPacketsPerSec, &network.Rates.TxPacketsPerSec
		}
		batch.Queue(`INSERT INTO net_metrics (time, host_id, iface, state, rx_bps, tx_bps, rx_pps, tx_pps, rx_bytes, tx_bytes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			at, hostID, network.Interface, network.Usage.State, rxBps, txBps, rxPps, txPps,
			float64(network.Usage.RxBytes), float64(network.Usage.TxBytes))
	}
	for _, temp := range data.Temperatures {
		batch.Queue("INSERT INTO temp_metrics (time, host_id, sensor, celsius) VALUES ($1, $2, $3, $4)",
			at, hostID, sensorName(temp), temp.Celsius)
	}
	for _, service := range data.Services {
		batch.Queue("INSERT INTO service_status (time, host_id, name, running) VALUES ($1, $2, $3, $4)",
			at, hostID, service.Name, service.Running)
	}
	batch.Queue("INSERT INTO process_snapshots (time, host_id, processes) VALUES ($1, $2, $3)", at, hostID, processes)

	// keep the newest snapshot even if an older one arrives late
	batch.Queue(`INSERT INTO host_latest (host_id, time, snapshot) VALUES ($1, $2, $3)
		ON CONFLICT (host_id) DO UPDATE SET time = excluded.time, snapshot = excluded.snapshot
		WHERE host_latest.time <= excluded.time`, hostID, at, snapshot)
	batch.Queue("UPDATE hosts SET last_seen = greatest(last_seen, now()) WHERE id = $1", hostID)

	return s.sendBatch(ctx, batch)
}

func queueHostMetrics(batch *pgx.Batch, at time.Time, hostID int64, data *monitor.MonitorData) {
	// pressure and tcp states are left null when the agent did not send them
	var psiCPU, psiMemory, psiMemoryFull, psiIO, psiIOFull *float64
	if p := data.Pressure; p != nil {
		psiCPU, psiMemory, psiIO = &p.CPU.Some.Avg10, &p.Memory.Some.Avg10, &p.IO.Some.Avg10
		if p.Memory.FullAvailable {
			psiMemoryFull = &p.Memory.Full.Avg10
		}
		if p.IO.FullAvailable {
			psiIOFull = &p.IO.Full.Avg10
		}
	}
	var established, timeWait, closeWait, listen, total *float64
	if t := data.TCPStates; t != nil {
		established, timeWait, closeWait, listen, total = floatPtr(t.Established), floatPtr(t.TimeWait), floatPtr(t.CloseWait), floatPtr(t.Listen), floatPtr(t.Total)
	}

	batch.Queue(`INSERT INTO host_metrics (time, host_id, cpu_pct, load1, load5, load15,
			mem_used_pct, mem_used_mib, mem_available_mib, mem_total_mib,
			swap_used_pct, swap_used_mib, swap_total_mib, uptime_seconds,
			psi_cpu_some, psi_memory_some, psi_memory_full, psi_io_some, psi_io_full,
			tcp_established, tcp_time_wait, tcp_close_wait, tcp_listen, tcp_total)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)`,
		at, hostID, float64(data.ProcUsage.LoadAvg), data.ProcUsage.Load1, data.ProcUsage.Load5, data.ProcUsage.Load15,
		data.Memory.PercentageUsed, float64(data.Memory.Used), float64(data.Memory.Available), float64(data.Memory.Total),
		data.Swap.PercentageUsed, float64(data.Swap.Used), float64(data.Swap.Total), data.System.UpTimeSeconds,
		psiCPU, psiMemory, psiMemoryFull, psiIO, psiIOFull,
		established, timeWait, closeWait, listen, total)
}

func floatPtr(v int) *float64 {
	f := float64(v)
	return &f
}

// SaveCustomMetric stores one custom metric value. The value must be numeric.
func (s *Store) SaveCustomMetric(ctx context.Context, metric *monitor.CustomMetric) error {
	value, err := strconv.ParseFloat(strings.TrimSpace(metric.Value), 64)
	if err != nil {
		return fmt.Errorf("%w: custom metric %s value %q is not a number", ErrInvalid, metric.Name, metric.Value)
	}
	at, err := parseUnixTime(metric.Time)
	if err != nil {
		return err
	}
	hostID, err := s.hostID(ctx, metric.ServerId)
	if err != nil {
		return err
	}

	batch := &pgx.Batch{}
	batch.Queue("INSERT INTO custom_metrics (time, host_id, name, unit, value) VALUES ($1, $2, $3, $4, $5)",
		at, hostID, metric.Name, metric.Unit, value)
	batch.Queue("UPDATE hosts SET last_seen = greatest(last_seen, now()) WHERE id = $1", hostID)
	return s.sendBatch(ctx, batch)
}

// sendBatch sends all statements in one round trip. Postgres runs a batch
// as one implicit transaction, so either all rows are written or none.
func (s *Store) sendBatch(ctx context.Context, batch *pgx.Batch) error {
	return s.pool.SendBatch(ctx, batch).Close()
}

func parseUnixTime(value string) (time.Time, error) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: bad unix time %q", ErrInvalid, value)
	}
	return time.Unix(seconds, 0), nil
}

// parsePercent turns "40%" into 40, and anything unparseable into null
func parsePercent(value string) *float64 {
	pct, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64)
	if err != nil {
		return nil
	}
	return &pct
}

// sensorName combines the chip and label, like "coretemp/Core 0"
func sensorName(temp monitor.Temperature) string {
	if len(temp.Label) == 0 {
		return temp.Name
	}
	return temp.Name + "/" + temp.Label
}
