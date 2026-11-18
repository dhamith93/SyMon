package store

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/proctrace"
)

// activitySnapshot is one 15 second interval: 30 execs and 28 exits, two
// of them with an error and one by a signal
func activitySnapshot(at time.Time, dropped int, abnormal ...proctrace.Exit) *proctrace.Summary {
	return &proctrace.Summary{
		From: at.Add(-15 * time.Second).Unix(), To: at.Unix(),
		Execs: 30, FailedExecs: 5, Exits: 28, NonZeroExits: 2, Signaled: 1, Dropped: dropped,
		Abnormal: abnormal,
	}
}

func TestProcessActivity(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}

	start := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	exit := func(at time.Time, name string, parent string) proctrace.Exit {
		return proctrace.Exit{Time: at.Unix(), Name: name, Pid: 4000, Ppid: 1, Parent: parent, LifetimeSeconds: 2.5}
	}
	at := func(i int) time.Time { return start.Add(time.Duration(i) * 15 * time.Second) }

	backup := exit(at(0), "backup.sh", "cron")
	backup.Status = 1
	orphan := backup
	orphan.Parent = ""
	again := backup
	again.Time = at(2).Unix()
	java := exit(at(1), "java", "systemd")
	java.Signal, java.OOM, java.LifetimeSeconds = 9, true, 86400
	node := exit(at(1), "node", "bash")
	node.Signal, node.CoreDumped = 11, true
	sleep := exit(at(2), "sleep", "bash")
	sleep.Signal = 9
	// no time of its own, so it takes the snapshot's
	nginx := exit(at(3), "nginx", "nginx")
	nginx.Signal, nginx.Time = 15, 0

	snapshots := []*proctrace.Summary{
		activitySnapshot(at(0), 0, backup, orphan),
		activitySnapshot(at(1), 0, java, node),
		activitySnapshot(at(2), 0, sleep, again),
		activitySnapshot(at(3), 3, nginx),
	}
	for i, activity := range snapshots {
		data := testSnapshot("web1", at(i))
		data.ProcessActivity = activity
		if err := st.SaveSnapshot(ctx, data); err != nil {
			t.Fatal(err)
		}
	}
	// a snapshot from an agent without the tracer adds nothing
	if err := st.SaveSnapshot(ctx, testSnapshot("web1", at(4))); err != nil {
		t.Fatal(err)
	}

	wantTotals := ProcessActivityTotals{Seconds: 60, Execs: 120, FailedExecs: 20, Exits: 112, NonZeroExits: 8, Signaled: 4, Dropped: 3, Listed: 7, OOM: 1, Crashes: 1}
	now := time.Now()

	t.Run("recent range", func(t *testing.T) {
		from := start.Add(-time.Minute)
		result, err := st.ProcessActivity(ctx, "web1", from, now)
		if err != nil {
			t.Fatal(err)
		}
		if result.Totals != wantTotals {
			t.Errorf("unexpected totals %+v, want %+v", result.Totals, wantTotals)
		}
		if !result.ExitsFrom.Equal(from) {
			t.Errorf("exits should be listed from the start of a recent range, got %v", result.ExitsFrom)
		}

		var order []string
		for _, g := range result.Groups {
			order = append(order, g.Name)
		}
		if want := []string{"java", "node", "sleep", "backup.sh", "nginx"}; !equalStrings(order, want) {
			t.Fatalf("unexpected group order %v, want %v", order, want)
		}
		java, backup := result.Groups[0], result.Groups[3]
		if !java.OOM || java.Signal != 9 || java.Count != 1 || java.Parent != "systemd" || java.LifetimeSeconds != 86400 {
			t.Errorf("unexpected OOM group %+v", java)
		}
		if backup.Count != 3 || backup.Status != 1 || backup.Parent != "cron" || !backup.First.Equal(at(0)) || !backup.Last.Equal(at(2)) {
			t.Errorf("unexpected backup group %+v", backup)
		}

		if len(result.Latest) != 7 || result.Latest[0].Name != "nginx" || !result.Latest[0].Time.Equal(at(3)) || result.Latest[0].Signal != 15 {
			t.Fatalf("unexpected latest exits %+v", result.Latest)
		}
		var node ProcessExit
		for i, e := range result.Latest {
			if i > 0 && e.Time.After(result.Latest[i-1].Time) {
				t.Errorf("latest exits should be newest first, got %+v", result.Latest)
			}
			if e.Name == "node" {
				node = e
			}
		}
		if !node.CoreDumped || node.Signal != 11 || node.Pid != 4000 || node.Ppid != 1 || node.LifetimeSeconds != 2.5 || !node.Time.Equal(at(1)) {
			t.Errorf("unexpected crash %+v", node)
		}
	})

	t.Run("rates per minute", func(t *testing.T) {
		series, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "proc_started", From: start.Add(-time.Minute), To: now})
		if err != nil {
			t.Fatal(err)
		}
		if series.Source != "raw" || len(series.Series) != 1 || len(series.Series[0].Points) != 4 {
			t.Fatalf("unexpected series %+v", series)
		}
		for _, p := range series.Series[0].Points {
			if math.Abs(p.Value-120) > 0.001 {
				t.Errorf("30 execs in 15 seconds should read 120 a minute, got %v", p.Value)
			}
		}
	})

	t.Run("rollups", func(t *testing.T) {
		refreshRollups(t, st, "process_activity_1m", "process_activity_1h")
		for _, span := range []struct {
			back   time.Duration
			source string
		}{{20 * time.Hour, "1m"}, {50 * 24 * time.Hour, "1h"}} {
			from := now.Add(-span.back)
			series, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "proc_ended", From: from, To: now})
			if err != nil {
				t.Fatal(err)
			}
			if series.Source != span.source || len(series.Series) != 1 {
				t.Fatalf("unexpected %s series %+v", span.source, series)
			}
			for _, p := range series.Series[0].Points {
				if math.Abs(p.Value-112) > 0.001 {
					t.Errorf("%s: 28 exits in 15 seconds should read 112 a minute, got %v", span.source, p.Value)
				}
			}

			result, err := st.ProcessActivity(ctx, "web1", from, now)
			if err != nil {
				t.Fatal(err)
			}
			if result.Totals != wantTotals {
				t.Errorf("%s: unexpected totals %+v", span.source, result.Totals)
			}
			// exits are only kept for the raw retention
			keptFrom := now.Add(-DefaultRetention.Raw)
			if span.source == "1h" && (result.ExitsFrom.Before(keptFrom.Add(-time.Minute)) || result.ExitsFrom.After(keptFrom.Add(time.Minute))) {
				t.Errorf("expected exits listed from %v, got %v", keptFrom, result.ExitsFrom)
			}
		}
	})

	t.Run("mistakes", func(t *testing.T) {
		if _, err := st.ProcessActivity(ctx, "web1", now, now.Add(-time.Hour)); !errors.Is(err, ErrInvalid) {
			t.Errorf("expected a backwards range to be refused, got %v", err)
		}
		if _, err := st.ProcessActivity(ctx, "ghost", start, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected an unknown host, got %v", err)
		}
		empty, err := st.ProcessActivity(ctx, "web1", start.Add(-3*time.Hour), start.Add(-2*time.Hour))
		if err != nil || empty.Totals != (ProcessActivityTotals{}) || len(empty.Groups) != 0 || len(empty.Latest) != 0 {
			t.Errorf("expected nothing before the first snapshot, got %+v, %v", empty, err)
		}
	})
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
