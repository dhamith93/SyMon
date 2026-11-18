package store

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/monitor"
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
}
