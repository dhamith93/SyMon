package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testStore returns a migrated store in a fresh schema, dropped after the
// test. It needs SYMON_TEST_DATABASE_URL pointing at a TimescaleDB database.
// The extension must already exist there unless the user is a superuser.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("SYMON_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SYMON_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	// in public, so dropping the test schema does not drop the extension
	if _, err := admin.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS timescaledb SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("symon_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	st := &Store{pool: pool, retention: DefaultRetention}

	t.Cleanup(func() {
		st.Close()
		// timescale's background jobs start on the new rollups right away and
		// can hold the catalog rows the drop needs, so try a few times
		for attempt := 1; ; attempt++ {
			_, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
			if err == nil {
				break
			}
			if attempt == 10 {
				t.Errorf("cannot drop %s: %v", schema, err)
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		admin.Close(ctx)
	})

	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}

func testSnapshot(host string, at time.Time) *monitor.MonitorData {
	return &monitor.MonitorData{
		UnixTime:  strconv.FormatInt(at.Unix(), 10),
		ServerId:  host,
		System:    monitor.System{HostName: host, OS: "Debian 13", UpTimeSeconds: 3600},
		Memory:    monitor.Memory{PercentageUsed: 42.5, Used: 7000, Available: 9000, Total: 16000, Unit: "MB"},
		Swap:      monitor.Swap{PercentageUsed: 1, Used: 20, Total: 2048, Unit: "MB"},
		ProcUsage: monitor.CPU{LoadAvg: 37, CoreAvg: []int{30, 44}, Load1: 0.5, Load5: 0.4, Load15: 0.3},
		Disk: []monitor.Disk{
			{FileSystem: "/dev/sda1", MountedOn: "/", Type: "ext4", Usage: monitor.DiskUsage{Size: 1000, Used: 400, Usage: "40%"}, Inodes: monitor.InodeUsage{Usage: "10%"}},
			{FileSystem: "/dev/sdb1", MountedOn: "/data", Type: "ext4", Usage: monitor.DiskUsage{Size: 1000, Used: 910, Usage: "91%"}, Inodes: monitor.InodeUsage{Usage: "5%"}},
		},
		Networks: []monitor.Network{
			{Interface: "eth0", Usage: monitor.NetworkUsage{RxBytes: 1000, TxBytes: 2000, State: "up"}, Rates: &monitor.NetworkRates{RxBytesPerSec: 100, TxBytesPerSec: 50}},
			{Interface: "eth1", Usage: monitor.NetworkUsage{State: "down"}},
		},
		Processes:    monitor.Processes{CPU: []monitor.Process{{Pid: 1, Name: "init"}}, Memory: []monitor.Process{}},
		Services:     []monitor.Service{{Name: "nginx", Running: true}},
		DiskIO:       []monitor.DiskIO{{Device: "sda", ReadBytesPerSec: 4096, UtilPercent: 12}},
		TCPStates:    &monitor.TCPStates{Established: 12, Listen: 4, Total: 20},
		Pressure:     &monitor.Pressure{CPU: monitor.ResourcePressure{Some: monitor.PressureMetric{Avg10: 1.5}}},
		Temperatures: []monitor.Temperature{{Name: "coretemp", Label: "Core 0", Celsius: 48}},
	}
}

func TestMigrateIsRepeatable(t *testing.T) {
	st := testStore(t)
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate failed: %v", err)
	}
}

func TestHosts(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddHost(ctx, "web1", "UTC"); !errors.Is(err, ErrHostExists) {
		t.Errorf("expected ErrHostExists, got %v", err)
	}
	if lastSeen, err := st.LastSeen(ctx, "web1"); err != nil || !lastSeen.IsZero() {
		t.Errorf("expected no last seen yet, got %v %v", lastSeen, err)
	}

	seen := time.Unix(1700000000, 0)
	if err := st.Heartbeat(ctx, "web1", seen); err != nil {
		t.Fatal(err)
	}
	// an older heartbeat does not move last seen back
	if err := st.Heartbeat(ctx, "web1", seen.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if lastSeen, _ := st.LastSeen(ctx, "web1"); !lastSeen.Equal(seen) {
		t.Errorf("last seen was %v, want %v", lastSeen, seen)
	}
	if err := st.Heartbeat(ctx, "nobody", seen); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown host, got %v", err)
	}

	if err := st.RemoveHost(ctx, "web1"); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveHost(ctx, "web1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound removing twice, got %v", err)
	}
}

func TestSnapshotFromUnknownHost(t *testing.T) {
	st := testStore(t)
	err := st.SaveSnapshot(context.Background(), testSnapshot("ghost", time.Now()))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFailedSnapshotWritesNothing(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot("web1", time.Now())
	// postgres rejects NUL bytes in text, so the disk insert fails after
	// the host metrics row was queued
	snapshot.Disk[0].MountedOn = "bad\x00mount"
	if err := st.SaveSnapshot(ctx, snapshot); err == nil {
		t.Fatal("expected the snapshot to fail")
	}

	var rows int
	if err := st.pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM host_metrics) + (SELECT count(*) FROM host_latest)").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("expected nothing written, found %d rows", rows)
	}
}

func TestSnapshotAndQueries(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	if err := st.SaveSnapshot(ctx, testSnapshot("web1", at)); err != nil {
		t.Fatal(err)
	}
	// an older snapshot arriving late does not replace the latest one
	if err := st.SaveSnapshot(ctx, testSnapshot("web1", at.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}

	t.Run("latest snapshot", func(t *testing.T) {
		snapshotTime, snapshot, err := st.LatestSnapshot(ctx, "web1")
		if err != nil {
			t.Fatal(err)
		}
		var data monitor.MonitorData
		if err := json.Unmarshal(snapshot, &data); err != nil {
			t.Fatal(err)
		}
		if !snapshotTime.Equal(at) || data.System.OS != "Debian 13" || len(data.Disk) != 2 {
			t.Errorf("unexpected snapshot at %v: %+v", snapshotTime, data)
		}
	})

	t.Run("fleet summary", func(t *testing.T) {
		fleet, err := st.FleetSummary(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(fleet) != 1 {
			t.Fatalf("expected one host, got %+v", fleet)
		}
		host := fleet[0]
		if host.CPUPct != 37 || host.DiskUsedPct != 91 || host.RxBps != 100 || host.TxBps != 50 || host.LastSeen.IsZero() {
			t.Errorf("unexpected summary: %+v", host)
		}
	})

	t.Run("raw series", func(t *testing.T) {
		result, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "cpu", From: at.Add(-5 * time.Minute), To: at.Add(5 * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if result.Source != "raw" || len(result.Series) != 1 || len(result.Series[0].Points) != 1 || result.Series[0].Points[0].Value != 37 {
			t.Errorf("unexpected result: %+v", result)
		}
	})

	t.Run("series per label", func(t *testing.T) {
		result, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "disk_used", From: at.Add(-5 * time.Minute), To: at.Add(5 * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Series) != 2 || result.Series[0].Label != "/" || result.Series[1].Label != "/data" {
			t.Fatalf("expected one series per mount, got %+v", result.Series)
		}
		filtered, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "disk_used", Label: "/data", From: at.Add(-5 * time.Minute), To: at.Add(5 * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if len(filtered.Series) != 1 || filtered.Series[0].Points[0].Value != 91 {
			t.Errorf("expected only /data, got %+v", filtered.Series)
		}
	})

	t.Run("rollup series includes recent data", func(t *testing.T) {
		// nothing is materialized yet, so this reads through to raw data
		result, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "temperature", From: at.Add(-24 * time.Hour), To: at.Add(time.Hour), Max: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Source != "1m" || len(result.Series) != 1 || result.Series[0].Label != "coretemp/Core 0" {
			t.Fatalf("unexpected result: %+v", result)
		}
		if points := result.Series[0].Points; len(points) != 2 || points[1].Value != 48 {
			t.Errorf("expected both snapshots, got %+v", points)
		}
	})

	t.Run("bad series queries", func(t *testing.T) {
		for _, q := range []SeriesQuery{
			{Host: "web1", Metric: "nope", From: at, To: at.Add(time.Hour)},
			{Host: "web1", Metric: "cpu", From: at, To: at},
			{Host: "web1", Metric: "load1", From: at, To: at.Add(time.Hour), Max: true},
		} {
			if _, err := st.QuerySeries(ctx, q); !errors.Is(err, ErrInvalid) {
				t.Errorf("%+v: expected ErrInvalid, got %v", q, err)
			}
		}
	})

	t.Run("processes at a time", func(t *testing.T) {
		snapshotTime, processes, err := st.Processes(ctx, "web1", at.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		var data monitor.Processes
		if err := json.Unmarshal(processes, &data); err != nil {
			t.Fatal(err)
		}
		if !snapshotTime.Equal(at) || len(data.CPU) != 1 || data.CPU[0].Name != "init" {
			t.Errorf("unexpected processes at %v: %+v", snapshotTime, data)
		}
		if _, _, err := st.Processes(ctx, "web1", at.Add(-2*time.Hour)); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound before the first snapshot, got %v", err)
		}
	})

	t.Run("latest values for alerts", func(t *testing.T) {
		checks := []struct {
			metric, target string
			want           float64
		}{
			{monitor.PROC_USAGE, "", 37},
			{monitor.MEMORY, "", 42.5},
			{monitor.DISKS, "/dev/sdb1", 91},
			{monitor.SERVICES, "nginx", 1},
		}
		for _, check := range checks {
			value, valueTime, err := st.LatestValue(ctx, "web1", check.metric, check.target, false)
			if err != nil || value != check.want || !valueTime.Equal(at) {
				t.Errorf("%s %s: got %v at %v, %v", check.metric, check.target, value, valueTime, err)
			}
		}
		if _, _, err := st.LatestValue(ctx, "web1", monitor.DISKS, "/dev/nope", false); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for an unknown disk, got %v", err)
		}
	})
}

func TestCustomMetrics(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Truncate(time.Second)
	metric := &monitor.CustomMetric{Name: "queue", Unit: "jobs", Value: " 12.5 ", ServerId: "web1", Time: strconv.FormatInt(at.Unix(), 10)}
	if err := st.SaveCustomMetric(ctx, metric); err != nil {
		t.Fatal(err)
	}

	value, _, err := st.LatestValue(ctx, "web1", "queue", "", true)
	if err != nil || value != 12.5 {
		t.Errorf("expected 12.5, got %v %v", value, err)
	}
	names, err := st.CustomMetricNames(ctx, "web1")
	if err != nil || len(names) != 1 || names[0] != "queue" {
		t.Errorf("expected [queue], got %v %v", names, err)
	}

	metric.Value = "lots"
	if err := st.SaveCustomMetric(ctx, metric); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for a non numeric value, got %v", err)
	}
}

func TestAlerts(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-time.Hour).Truncate(time.Second)

	if open, err := st.OpenAlert(ctx, "web1", "Disk", monitor.DISKS, "/dev/sda1"); err != nil || open != nil {
		t.Fatalf("expected no open alert, got %+v %v", open, err)
	}
	id, err := st.CreateAlert(ctx, Alert{Host: "web1", Rule: "Disk", Metric: monitor.DISKS, Target: "/dev/sda1", Severity: 1, Value: 85, StartedAt: start})
	if err != nil {
		t.Fatal(err)
	}
	// only one open alert per host, rule and target
	if _, err := st.CreateAlert(ctx, Alert{Host: "web1", Rule: "Disk", Metric: monitor.DISKS, Target: "/dev/sda1", Severity: 1, StartedAt: start}); err == nil {
		t.Error("expected a second open alert to be rejected")
	}

	if err := st.UpdateAlert(ctx, id, 2, 95, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	open, err := st.OpenAlert(ctx, "web1", "Disk", monitor.DISKS, "/dev/sda1")
	if err != nil || open == nil || open.Severity != 2 || open.Value != 95 {
		t.Fatalf("expected an open critical alert, got %+v %v", open, err)
	}

	fleet, _ := st.FleetSummary(ctx)
	if len(fleet) != 1 || fleet[0].ActiveAlerts != 1 {
		t.Errorf("expected one active alert in the summary, got %+v", fleet)
	}

	if err := st.ResolveAlert(ctx, id, 50, start.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if open, _ := st.OpenAlert(ctx, "web1", "Disk", monitor.DISKS, "/dev/sda1"); open != nil {
		t.Errorf("expected the alert to be resolved, got %+v", open)
	}

	all, err := st.Alerts(ctx, AlertFilter{Host: "web1"})
	if err != nil || len(all) != 1 || all[0].ResolvedAt == nil {
		t.Errorf("expected one resolved alert, got %+v %v", all, err)
	}
	openOnly, err := st.Alerts(ctx, AlertFilter{OpenOnly: true})
	if err != nil || len(openOnly) != 0 {
		t.Errorf("expected no open alerts, got %+v %v", openOnly, err)
	}

	// resolved long ago, so the purge removes it
	if _, err := st.pool.Exec(ctx, "UPDATE alerts SET resolved_at = now() - interval '400 days'"); err != nil {
		t.Fatal(err)
	}
	if deleted, err := st.PurgeResolvedAlerts(ctx); err != nil || deleted != 1 {
		t.Errorf("expected one purged alert, got %d %v", deleted, err)
	}
}

func TestPolicies(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	// running it twice replaces the policies instead of failing
	for i := 0; i < 2; i++ {
		if err := st.ApplyRetention(ctx); err != nil {
			t.Fatal(err)
		}
	}

	var schema string
	if err := st.pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	rows, err := st.pool.Query(ctx, `
		SELECT j.proc_name, count(*) FROM timescaledb_information.jobs j
		LEFT JOIN timescaledb_information.continuous_aggregates c
		  ON c.materialization_hypertable_schema = j.hypertable_schema
		 AND c.materialization_hypertable_name = j.hypertable_name
		WHERE j.hypertable_schema = $1 OR c.view_schema = $1
		GROUP BY j.proc_name`, schema)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			t.Fatal(err)
		}
		counts[name] = count
	}

	want := map[string]int{
		"policy_retention":                    len(rawTables) + len(minuteTables) + len(hourTables),
		"policy_compression":                  len(rawTables),
		"policy_refresh_continuous_aggregate": len(minuteTables) + len(hourTables),
	}
	for name, count := range want {
		if counts[name] != count {
			t.Errorf("%s: got %d jobs, want %d (all: %v)", name, counts[name], count, counts)
		}
	}
}

// data that arrives hours late, like after a collector outage, must still
// reach the rollups the charts read
func TestLateDataIsRolledUp(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}

	// the minute rollup policy, run by hand the way the scheduler would
	var job int
	err := st.pool.QueryRow(ctx, `
		SELECT j.job_id FROM timescaledb_information.jobs j
		LEFT JOIN timescaledb_information.continuous_aggregates c
		  ON c.materialization_hypertable_schema = j.hypertable_schema
		 AND c.materialization_hypertable_name = j.hypertable_name
		WHERE j.proc_name = 'policy_refresh_continuous_aggregate'
		  AND ((j.hypertable_schema = current_schema() AND j.hypertable_name = 'host_metrics_1m')
		    OR (c.view_schema = current_schema() AND c.view_name = 'host_metrics_1m'))`).Scan(&job)
	if err != nil {
		t.Fatal(err)
	}
	runJob := func() {
		t.Helper()
		if _, err := st.pool.Exec(ctx, "CALL run_job($1)", job); err != nil {
			t.Fatal(err)
		}
	}

	// the collector has been running, so the rollup is current up to a
	// recent snapshot
	if err := st.SaveSnapshot(ctx, testSnapshot("web1", time.Now().Add(-10*time.Minute))); err != nil {
		t.Fatal(err)
	}
	runJob()
	// then a snapshot from 5 hours ago arrives
	at := time.Now().Add(-5 * time.Hour).Truncate(time.Minute)
	if err := st.SaveSnapshot(ctx, testSnapshot("web1", at)); err != nil {
		t.Fatal(err)
	}
	runJob()

	result, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "cpu", From: at.Add(-time.Hour), To: at.Add(time.Hour), MaxPoints: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "1m" || len(result.Series) != 1 || result.Series[0].Points[0].Value != 37 {
		t.Errorf("expected the late point from the minute rollup, got %+v", result)
	}
}
