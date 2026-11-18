package store

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/proctrace"
)

func TestDiskLatency(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}

	// on a two hour boundary, so every rollup puts the snapshots in one bucket
	start := time.Now().Add(-3 * time.Hour).Truncate(2 * time.Hour)
	at := func(i int) time.Time { return start.Add(time.Duration(i) * 15 * time.Second) }
	sda := []monitor.DiskLatencyCounts{
		{Device: "sda", Reads: 100, ReadMs: 200, Writes: 50, WriteMs: 500, QueueMs: 15000},
		{Device: "sda", Reads: 300, ReadMs: 300, Writes: 50, WriteMs: 500, QueueMs: 15000},
		// no reads, so no read latency
		{Device: "sda", Writes: 50, WriteMs: 500, QueueMs: 15000},
		{Device: "sda", Reads: 100, ReadMs: 1000, Writes: 50, WriteMs: 500, QueueMs: 15000},
	}
	for i, counts := range sda {
		data := testSnapshot("web1", at(i))
		// sdb is idle the whole time
		data.DiskLatency = &monitor.DiskLatency{Seconds: 15, Disks: []monitor.DiskLatencyCounts{counts, {Device: "sdb"}}}
		if err := st.SaveSnapshot(ctx, data); err != nil {
			t.Fatal(err)
		}
	}
	// an agent without the counters adds nothing
	if err := st.SaveSnapshot(ctx, testSnapshot("web1", at(4))); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	type wantSeries struct {
		label  string
		values []float64
	}
	check := func(t *testing.T, metric string, from time.Time, to time.Time, source string, want []wantSeries) {
		t.Helper()
		result, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: metric, From: from, To: to})
		if err != nil {
			t.Fatal(err)
		}
		if result.Source != source {
			t.Fatalf("%s: expected %s data, got %s", metric, source, result.Source)
		}
		if len(result.Series) != len(want) {
			t.Fatalf("%s: expected %d series, got %+v", metric, len(want), result.Series)
		}
		for i, w := range want {
			got := result.Series[i]
			if got.Label != w.label || len(got.Points) != len(w.values) {
				t.Errorf("%s: expected %s with %d points, got %+v", metric, w.label, len(w.values), got)
				continue
			}
			for j, p := range got.Points {
				if math.Abs(p.Value-w.values[j]) > 0.001 {
					t.Errorf("%s %s point %d: expected %v, got %v", metric, w.label, j, w.values[j], p.Value)
				}
			}
		}
	}

	t.Run("raw", func(t *testing.T) {
		// a short range, so every snapshot is its own point
		from, to := start.Add(-time.Minute), start.Add(5*time.Minute)
		// an idle interval is a gap, and an idle disk has no series
		check(t, "disk_read_latency", from, to, "raw", []wantSeries{{"/dev/sda", []float64{2, 1, 10}}})
		check(t, "disk_write_latency", from, to, "raw", []wantSeries{{"/dev/sda", []float64{10, 10, 10, 10}}})
		// 15 seconds of IO time in every 15 second interval
		check(t, "disk_queue", from, to, "raw", []wantSeries{{"/dev/sda", []float64{1, 1, 1, 1}}, {"/dev/sdb", []float64{0, 0, 0, 0}}})
	})

	t.Run("rollups", func(t *testing.T) {
		refreshRollups(t, st, "disk_latency_1m", "disk_latency_1h")
		for _, span := range []struct {
			back   time.Duration
			source string
		}{{20 * time.Hour, "1m"}, {50 * 24 * time.Hour, "1h"}} {
			from := now.Add(-span.back)
			// 1500 ms over 500 reads, not the average of 2, 1 and 10
			check(t, "disk_read_latency", from, now, span.source, []wantSeries{{"/dev/sda", []float64{3}}})
			check(t, "disk_write_latency", from, now, span.source, []wantSeries{{"/dev/sda", []float64{10}}})
			check(t, "disk_queue", from, now, span.source, []wantSeries{{"/dev/sda", []float64{1}}, {"/dev/sdb", []float64{0}}})
		}
	})

	t.Run("alert rules", func(t *testing.T) {
		// the newest interval: 1000 ms over 100 reads, 500 ms over 50 writes
		for metric, want := range map[string]float64{monitor.DISK_READ_LATENCY: 10, monitor.DISK_WRITE_LATENCY: 10} {
			value, when, err := st.LatestValue(ctx, "web1", metric, "/dev/sda", false)
			if err != nil || value != want || !when.Equal(at(3)) {
				t.Errorf("%s: expected %v at %v, got %v at %v, %v", metric, want, at(3), value, when, err)
			}
		}
		// an idle disk has no latency to alert on
		if _, _, err := st.LatestValue(ctx, "web1", monitor.DISK_READ_LATENCY, "/dev/sdb", false); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected no data for an idle disk, got %v", err)
		}
	})
}

func TestDiskLatencyBuckets(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "db1", "UTC"); err != nil {
		t.Fatal(err)
	}

	buckets := func(set map[int]uint64) []uint64 {
		counts := make([]uint64, proctrace.LatencyBuckets)
		for i, c := range set {
			counts[i] = c
		}
		return counts
	}
	// on a two hour boundary, so every rollup puts the snapshots in one step
	start := time.Now().Add(-3 * time.Hour).Truncate(2 * time.Hour)
	at := func(i int) time.Time { return start.Add(time.Duration(i) * 15 * time.Second) }
	for i := range 4 {
		data := testSnapshot("db1", at(i))
		data.DiskLatency = &monitor.DiskLatency{Seconds: 15, Disks: []monitor.DiskLatencyCounts{
			// 90 reads from 32 to 64 µs and 10 from 8 to 16 ms, 20 writes
			{Device: "sda", Reads: 100, Writes: 20, ReadBuckets: buckets(map[int]uint64{2: 90, 10: 10}), WriteBuckets: buckets(map[int]uint64{7: 20})},
			// a partition has no buckets
			{Device: "sda1", Reads: 100},
			{Device: "sdb", Writes: 3, WriteBuckets: buckets(map[int]uint64{5: 3})},
			// a disk the tracer saw nothing on
			{Device: "sdc", ReadBuckets: buckets(nil)},
		}}
		if err := st.SaveSnapshot(ctx, data); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now()
	// the same share of fast and slow reads in every step
	p50, p95, p99 := 0.032+0.032*50/90, 8.192*1.5, 8.192*1.9
	check := func(t *testing.T, result DiskLatencyResult, device string, steps int, quantiles ...float64) {
		t.Helper()
		if strings.Join(result.Devices, ",") != "/dev/sda,/dev/sdb" || result.Device != device || len(result.Steps) != steps {
			t.Fatalf("expected %d steps of %s, got %+v", steps, device, result)
		}
		for i, step := range result.Steps {
			got := []float64{step.P50, step.P95, step.P99}
			for j := range quantiles {
				if math.Abs(got[j]-quantiles[j]) > 1e-6 {
					t.Errorf("step %d: expected quantiles %v, got %v", i, quantiles, got)
					break
				}
			}
		}
	}

	t.Run("raw", func(t *testing.T) {
		from, to := start.Add(-time.Minute), start.Add(5*time.Minute)
		result, err := st.DiskLatency(ctx, "db1", "", "read", from, to)
		if err != nil {
			t.Fatal(err)
		}
		// the busiest disk when none is asked for
		check(t, result, "/dev/sda", 4, p50, p95, p99)
		if b := result.Steps[0].Buckets; len(b) != proctrace.LatencyBuckets || b[2] != 90 || b[10] != 10 || !result.Steps[0].Time.Equal(at(0)) {
			t.Errorf("unexpected first step %+v", result.Steps[0])
		}

		result, err = st.DiskLatency(ctx, "db1", "/dev/sdb", "write", from, to)
		if err != nil {
			t.Fatal(err)
		}
		check(t, result, "/dev/sdb", 4, 0.256+0.256*1.5/3)

		// a disk without a histogram picks the busiest
		result, err = st.DiskLatency(ctx, "db1", "/dev/sda1", "read", from, to)
		if err != nil {
			t.Fatal(err)
		}
		check(t, result, "/dev/sda", 4, p50)
	})

	t.Run("rollups", func(t *testing.T) {
		refreshRollups(t, st, "disk_latency_buckets_1m", "disk_latency_buckets_1h")
		for _, back := range []time.Duration{20 * time.Hour, 50 * 24 * time.Hour} {
			result, err := st.DiskLatency(ctx, "db1", "/dev/sda", "read", now.Add(-back), now)
			if err != nil {
				t.Fatal(err)
			}
			check(t, result, "/dev/sda", 1, p50, p95, p99)
			if b := result.Steps[0].Buckets; b[2] != 360 || b[10] != 40 {
				t.Errorf("expected the four snapshots added up, got %v", b)
			}
		}
	})

	t.Run("mistakes", func(t *testing.T) {
		if _, err := st.DiskLatency(ctx, "db1", "", "trim", start, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("expected an unknown op to be refused, got %v", err)
		}
		if _, err := st.DiskLatency(ctx, "db1", "", "read", now, start); !errors.Is(err, ErrInvalid) {
			t.Errorf("expected a backwards range to be refused, got %v", err)
		}
		if _, err := st.DiskLatency(ctx, "ghost", "", "read", start, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected an unknown host, got %v", err)
		}
		empty, err := st.DiskLatency(ctx, "db1", "", "read", start.Add(-3*time.Hour), start.Add(-2*time.Hour))
		if err != nil || empty.Devices == nil || len(empty.Devices) != 0 || empty.Device != "" || len(empty.Steps) != 0 {
			t.Errorf("expected nothing before the first snapshot, got %+v, %v", empty, err)
		}
	})
}
