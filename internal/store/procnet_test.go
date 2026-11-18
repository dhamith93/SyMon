package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/proctrace"
)

// networkSnapshot is one 15 second interval ending at at
func networkSnapshot(at time.Time, rx uint64, tx uint64, programs ...proctrace.ProgramTraffic) *proctrace.NetSummary {
	return &proctrace.NetSummary{From: at.Add(-15 * time.Second).Unix(), To: at.Unix(), RxBytes: rx, TxBytes: tx, Programs: programs}
}

func TestProcessNetwork(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	for _, host := range []string{"web1", "web2"} {
		if err := st.AddHost(ctx, host, "UTC"); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	at := func(i int) time.Time { return start.Add(time.Duration(i) * 15 * time.Second) }
	nginx := proctrace.ProgramTraffic{Name: "nginx", RxBytes: 1500, TxBytes: 15000}
	curl := proctrace.ProgramTraffic{Name: "curl", RxBytes: 30000, TxBytes: 300}
	// a program without a name is left in the rest
	nameless := proctrace.ProgramTraffic{RxBytes: 60}

	snapshots := []*proctrace.NetSummary{
		// 300 received and 100 sent by programs that are not listed
		networkSnapshot(at(0), 31800, 15400, nginx, curl),
		networkSnapshot(at(1), 1500, 15000, nginx),
		networkSnapshot(at(2), 1600, 15000, nginx, nameless),
		// a quiet interval still has its rest row
		networkSnapshot(at(3), 0, 0),
	}
	for i, network := range snapshots {
		data := testSnapshot("web1", at(i))
		data.ProcessNetwork = network
		if err := st.SaveSnapshot(ctx, data); err != nil {
			t.Fatal(err)
		}
	}
	// an agent without the counters adds nothing
	if err := st.SaveSnapshot(ctx, testSnapshot("web1", at(4))); err != nil {
		t.Fatal(err)
	}

	// nine programs, so two of them are folded into the rest
	var many []proctrace.ProgramTraffic
	for i := 1; i <= 9; i++ {
		many = append(many, proctrace.ProgramTraffic{Name: fmt.Sprintf("p%d", i), RxBytes: uint64(10-i) * 100})
	}
	data := testSnapshot("web2", at(0))
	data.ProcessNetwork = networkSnapshot(at(0), 4500, 0, many...)
	if err := st.SaveSnapshot(ctx, data); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	checkTotals := func(t *testing.T, result ProcessNetworkResult, peaks bool) {
		t.Helper()
		if result.RxBytes != 34900 || result.TxBytes != 45400 || result.Seconds != 60 {
			t.Errorf("unexpected totals %+v", result)
		}
		if len(result.Programs) != 2 {
			t.Fatalf("expected nginx and curl, got %+v", result.Programs)
		}
		first, second := result.Programs[0], result.Programs[1]
		if first.Name != "nginx" || first.RxBytes != 4500 || first.TxBytes != 45000 {
			t.Errorf("expected nginx first, got %+v", first)
		}
		if second.Name != "curl" || second.RxBytes != 30000 || second.TxBytes != 300 {
			t.Errorf("expected curl second, got %+v", second)
		}
		// the fastest interval: 1500 and 15000 bytes in 15 seconds
		if peaks && (first.RxPeak != 100 || first.TxPeak != 1000 || second.RxPeak != 2000 || second.TxPeak != 20) {
			t.Errorf("unexpected peaks %+v", result.Programs)
		}
	}

	t.Run("recent range", func(t *testing.T) {
		result, err := st.ProcessNetwork(ctx, "web1", start.Add(-time.Minute), now)
		if err != nil {
			t.Fatal(err)
		}
		checkTotals(t, result, true)
	})

	type wantSeries struct {
		label  string
		values []float64
	}
	checkSeries := func(t *testing.T, got []Series, want []wantSeries) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("expected %d series, got %+v", len(want), got)
		}
		for i, w := range want {
			if got[i].Label != w.label || len(got[i].Points) != len(w.values) {
				t.Errorf("series %d: expected %s with %d points, got %+v", i, w.label, len(w.values), got[i])
				continue
			}
			for j, p := range got[i].Points {
				if math.Abs(p.Value-w.values[j]) > 0.001 {
					t.Errorf("%s point %d: expected %v bytes a second, got %v", w.label, j, w.values[j], p.Value)
				}
			}
		}
	}

	t.Run("rates per second", func(t *testing.T) {
		series, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "proc_net_rx", From: start.Add(-time.Minute), To: now})
		if err != nil {
			t.Fatal(err)
		}
		if series.Source != "raw" {
			t.Fatalf("expected raw data, got %s", series.Source)
		}
		// each raw row is 15 seconds of traffic, the rest comes last
		checkSeries(t, series.Series, []wantSeries{
			{"curl", []float64{2000}},
			{"nginx", []float64{100, 100, 100}},
			{otherLabel, []float64{20, 0, 100.0 / 15, 0}},
		})
	})

	t.Run("quiet programs folded", func(t *testing.T) {
		series, err := st.QuerySeries(ctx, SeriesQuery{Host: "web2", Metric: "proc_net_rx", From: start.Add(-time.Minute), To: now})
		if err != nil {
			t.Fatal(err)
		}
		var want []wantSeries
		for i := 1; i <= 7; i++ {
			want = append(want, wantSeries{fmt.Sprintf("p%d", i), []float64{float64(10-i) * 100 / 15}})
		}
		// p8 and p9 with 200 and 100 bytes
		want = append(want, wantSeries{otherLabel, []float64{20}})
		checkSeries(t, series.Series, want)
	})

	t.Run("rollups", func(t *testing.T) {
		refreshRollups(t, st, "process_network_1m", "process_network_1h")
		for _, span := range []struct {
			back   time.Duration
			source string
			// curl's 30000 bytes over the bucket
			curlRate float64
		}{{20 * time.Hour, "1m", 30000.0 / 120}, {50 * 24 * time.Hour, "1h", 30000.0 / 7200}} {
			from := now.Add(-span.back)
			series, err := st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "proc_net_rx", From: from, To: now})
			if err != nil {
				t.Fatal(err)
			}
			if series.Source != span.source || len(series.Series) != 3 || series.Series[0].Label != "curl" || len(series.Series[0].Points) != 1 {
				t.Fatalf("unexpected %s series %+v", span.source, series)
			}
			if got := series.Series[0].Points[0].Value; math.Abs(got-span.curlRate) > 0.001 {
				t.Errorf("%s: expected curl at %v bytes a second, got %v", span.source, span.curlRate, got)
			}

			result, err := st.ProcessNetwork(ctx, "web1", from, now)
			if err != nil {
				t.Fatal(err)
			}
			checkTotals(t, result, true)
		}
	})

	t.Run("mistakes", func(t *testing.T) {
		if _, err := st.ProcessNetwork(ctx, "web1", now, now.Add(-time.Hour)); !errors.Is(err, ErrInvalid) {
			t.Errorf("expected a backwards range to be refused, got %v", err)
		}
		if _, err := st.ProcessNetwork(ctx, "ghost", start, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected an unknown host, got %v", err)
		}
		empty, err := st.ProcessNetwork(ctx, "web1", start.Add(-3*time.Hour), start.Add(-2*time.Hour))
		if err != nil || empty.RxBytes != 0 || empty.Seconds != 0 || empty.Programs == nil || len(empty.Programs) != 0 {
			t.Errorf("expected nothing before the first snapshot, got %+v, %v", empty, err)
		}
	})
}
