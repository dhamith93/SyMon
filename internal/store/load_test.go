package store

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// These run only with SYMON_LOAD_TEST=1, since they take a few minutes.
// SYMON_LOAD_P99_MS changes the ingest latency limit.

func loadStore(t *testing.T) *Store {
	t.Helper()
	if os.Getenv("SYMON_LOAD_TEST") != "1" {
		t.Skip("SYMON_LOAD_TEST is not set")
	}
	return testStore(t)
}

// 200 agents sending every 15s, for three rounds
func TestLoadIngest(t *testing.T) {
	st := loadStore(t)
	ctx := context.Background()
	const hosts, rounds, interval = 200, 3, 15 * time.Second
	for i := 0; i < hosts; i++ {
		if err := st.AddHost(ctx, fmt.Sprintf("host%03d", i), "UTC"); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	var latencies []time.Duration
	for round := 0; round < rounds; round++ {
		at := time.Now().Add(-time.Duration(rounds-round) * interval)
		var wg sync.WaitGroup
		for i := 0; i < hosts; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				time.Sleep(time.Duration(rand.Int63n(int64(interval))))
				started := time.Now()
				if err := st.SaveSnapshot(ctx, testSnapshot(fmt.Sprintf("host%03d", i), at)); err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				latencies = append(latencies, time.Since(started))
				mu.Unlock()
			}(i)
		}
		wg.Wait()
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)/2]
	p99 := latencies[len(latencies)*99/100]
	t.Logf("%d snapshots, p50 %v, p99 %v", len(latencies), p50, p99)

	// a commit waits for the database's disk, so the limit depends on the
	// hardware. 50ms fits a collector next to its database on an SSD.
	limit := 50 * time.Millisecond
	if ms, err := strconv.Atoi(os.Getenv("SYMON_LOAD_P99_MS")); err == nil {
		limit = time.Duration(ms) * time.Millisecond
	}
	if p99 > limit {
		t.Errorf("p99 %v is over %v", p99, limit)
	}
}

// a host sending every 15s for 30 days, then a 30 day chart
func TestLoadQuery30Days(t *testing.T) {
	st := loadStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	hostID, err := st.hostID(ctx, "web1")
	if err != nil {
		t.Fatal(err)
	}

	end := time.Now().Truncate(time.Second)
	start := end.Add(-30 * 24 * time.Hour)
	var rows [][]any
	for at := start; at.Before(end); at = at.Add(15 * time.Second) {
		rows = append(rows, []any{at, hostID, rand.Float64() * 100, rand.Float64() * 100})
	}
	copied, err := st.pool.CopyFrom(ctx, pgx.Identifier{"host_metrics"}, []string{"time", "host_id", "cpu_pct", "mem_used_pct"}, pgx.CopyFromRows(rows))
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"host_metrics_1m", "host_metrics_1h"} {
		if _, err := st.pool.Exec(ctx, "CALL refresh_continuous_aggregate($1, $2::timestamptz, $3::timestamptz)", view, start, end); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("inserted %d rows", copied)

	from := end.Add(-30*24*time.Hour + time.Minute)
	var best time.Duration
	var result SeriesResult
	for i := 0; i < 5; i++ {
		started := time.Now()
		result, err = st.QuerySeries(ctx, SeriesQuery{Host: "web1", Metric: "cpu", From: from, To: end})
		if err != nil {
			t.Fatal(err)
		}
		if took := time.Since(started); best == 0 || took < best {
			best = took
		}
	}

	points := len(result.Series[0].Points)
	t.Logf("30 day query: source %s, step %v, %d points, best of 5 %v", result.Source, result.Step, points, best)
	if points > defaultMaxPoints+1 {
		t.Errorf("%d points is over the limit", points)
	}
	if best > 100*time.Millisecond {
		t.Errorf("query took %v, over 100ms", best)
	}
}
