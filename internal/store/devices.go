package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/snmp"
	"github.com/jackc/pgx/v5"
)

// ErrDeviceTaken is a report from a host other than the one that polls the
// device
var ErrDeviceTaken = errors.New("another host polls this device, remove the device on the dashboard to move its poller")

// Device is an SNMP device with the newest report that could read it
type Device struct {
	Name    string
	Kind    string
	Address string
	// Host is the host whose agent polls it, empty once that host is removed
	Host string
	// Interval is how often it is polled
	Interval time.Duration
	// ReportedAt is the last report, Error why it could not read the device
	ReportedAt time.Time
	Error      string
	State      snmp.DeviceReport
}

// staleAfter is how many polls a device may miss before its state is too
// old to alert on. A report with an error is still a poll: an unreachable
// device is not stale, its agent stopping makes it so.
const staleAfter = 3

func (d Device) Stale(now time.Time) bool {
	return now.Sub(d.ReportedAt) > staleAfter*d.Interval
}

const (
	maxDeviceName         = 64
	defaultDeviceInterval = 60
)

// SaveDeviceReport keeps what a host's agent read of a device. The first
// report makes the device. A report that could not read the device only
// records the error, so the last state stays.
func (s *Store) SaveDeviceReport(ctx context.Context, host string, report snmp.DeviceReport) error {
	report.Device = strings.TrimSpace(report.Device)
	if report.Device == "" || len(report.Device) > maxDeviceName || strings.Contains(report.Device, "/") {
		return fmt.Errorf("%w: a device needs a name of up to %d characters without /", ErrInvalid, maxDeviceName)
	}
	if report.IntervalSeconds <= 0 {
		report.IntervalSeconds = defaultDeviceInterval
	}
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return err
	}
	at := time.Unix(report.Time, 0)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var deviceID int64
	var owner *int64
	var prev snmp.DeviceReport
	err = tx.QueryRow(ctx, `SELECT id, host_id, state FROM devices WHERE name = $1 FOR UPDATE`, report.Device).Scan(&deviceID, &owner, &prev)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		saved := report
		if report.Error != "" {
			// nothing read yet
			saved = snmp.DeviceReport{Device: report.Device, Kind: report.Kind, Address: report.Address, IntervalSeconds: report.IntervalSeconds}
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO devices (name, kind, address, host_id, state, error, interval_seconds, reported_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (name) DO NOTHING RETURNING id`,
			report.Device, report.Kind, report.Address, hostID, saved, report.Error, report.IntervalSeconds, at).Scan(&deviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			// another report made it first
			return ErrDeviceTaken
		}
		if err != nil {
			return err
		}
		if report.Error == "" {
			if err := saveDeviceMetrics(ctx, tx, deviceID, report); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	case err != nil:
		return err
	case owner != nil && *owner != hostID:
		return ErrDeviceTaken
	}

	if report.Error != "" {
		if _, err := tx.Exec(ctx, `UPDATE devices SET host_id = $2, kind = $3, address = $4, error = $5, interval_seconds = $6, reported_at = $7 WHERE id = $1`,
			deviceID, hostID, report.Kind, report.Address, report.Error, report.IntervalSeconds, at); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if report.Time <= prev.Time {
		// an older report arriving late
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `UPDATE devices SET host_id = $2, kind = $3, address = $4, state = $5, error = '', interval_seconds = $6, reported_at = $7 WHERE id = $1`,
		deviceID, hostID, report.Kind, report.Address, report, report.IntervalSeconds, at); err != nil {
		return err
	}
	if err := saveDeviceMetrics(ctx, tx, deviceID, report); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func saveDeviceMetrics(ctx context.Context, tx pgx.Tx, deviceID int64, report snmp.DeviceReport) error {
	at := time.Unix(report.Time, 0)
	batch := &pgx.Batch{}
	for _, v := range report.Values {
		if math.IsNaN(v.Value) || math.IsInf(v.Value, 0) {
			continue
		}
		batch.Queue(`INSERT INTO device_metrics (time, device_id, metric, label, unit, value) VALUES ($1, $2, $3, $4, $5, $6)`,
			at, deviceID, v.Metric, v.Label, v.Unit, v.Value)
	}
	if batch.Len() == 0 {
		return nil
	}
	return tx.SendBatch(ctx, batch).Close()
}

const deviceColumns = `d.name, d.kind, d.address, coalesce(h.name, ''), d.interval_seconds, d.reported_at, d.error, d.state
	FROM devices d LEFT JOIN hosts h ON h.id = d.host_id`

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	var interval int
	err := row.Scan(&d.Name, &d.Kind, &d.Address, &d.Host, &interval, &d.ReportedAt, &d.Error, &d.State)
	d.Interval = time.Duration(interval) * time.Second
	return d, err
}

// Devices lists the devices by name
func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+deviceColumns+` ORDER BY d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *Store) Device(ctx context.Context, name string) (Device, error) {
	d, err := scanDevice(s.pool.QueryRow(ctx, `SELECT `+deviceColumns+` WHERE d.name = $1`, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

// RemoveDevice forgets a device, so another host may poll it. Its series
// stay until retention drops them.
func (s *Store) RemoveDevice(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE name = $1`, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// device metrics are one table for every metric, so the series definition
// is fixed and the metric is a value, never SQL
var deviceSeriesDef = seriesDef{table: "device_metrics", column: "value", label: "label", hasMax: true}

// DeviceSeries returns one of a device's metrics over a range, one series
// per label, from the finest data that gives at most maxPoints points.
// labels keeps only those, like a few ports of a big switch, none is all.
func (s *Store) DeviceSeries(ctx context.Context, device string, metric string, labels []string, from time.Time, to time.Time, maxPoints int) (SeriesResult, error) {
	if strings.TrimSpace(metric) == "" {
		return SeriesResult{}, fmt.Errorf("%w: a metric is needed, like if_rx_bps", ErrInvalid)
	}
	if !to.After(from) {
		return SeriesResult{}, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	if maxPoints <= 0 {
		maxPoints = defaultMaxPoints
	}
	var deviceID int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM devices WHERE name = $1`, device).Scan(&deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SeriesResult{}, ErrNotFound
	}
	if err != nil {
		return SeriesResult{}, err
	}

	source, step := pickSource(deviceSeriesDef, from, to, time.Now(), maxPoints, s.retention)
	table, timeColumn := deviceSeriesDef.table, "time"
	if source != "raw" {
		table, timeColumn = table+"_"+source, "bucket"
	}
	args := []any{fmt.Sprintf("%d seconds", int64(step.Seconds())), deviceID, metric, from, to}
	onlyLabels := ""
	if len(labels) > 0 {
		onlyLabels = "AND label = ANY($6)"
		args = append(args, labels)
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT time_bucket($1::interval, %[1]s) AS t, label, avg(value) AS v
		FROM %[2]s
		WHERE device_id = $2 AND metric = $3 AND %[1]s >= $4 AND %[1]s < $5 %[3]s
		GROUP BY t, label
		ORDER BY label, t`, timeColumn, table, onlyLabels), args...)
	if err != nil {
		return SeriesResult{}, err
	}
	defer rows.Close()
	series, err := readSeries(rows)
	if err != nil {
		return SeriesResult{}, err
	}
	return SeriesResult{Source: source, Step: step, Series: series}, nil
}
