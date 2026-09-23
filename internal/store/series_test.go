package store

import (
	"strings"
	"testing"
	"time"
)

func TestPickSource(t *testing.T) {
	now := time.Unix(1700000000, 0)
	host := seriesDefs["cpu"]
	tests := []struct {
		name       string
		def        seriesDef
		span       time.Duration
		ago        time.Duration
		wantSource string
		wantStep   time.Duration
	}{
		{"last hour", host, time.Hour, time.Hour, "raw", 4 * time.Second},
		{"last day", host, 24 * time.Hour, 24 * time.Hour, "1m", 2 * time.Minute},
		{"last week", host, 7 * 24 * time.Hour, 7 * 24 * time.Hour, "1m", 11 * time.Minute},
		{"last 30 days", host, 30 * 24 * time.Hour, 29 * 24 * time.Hour, "1m", 44 * time.Minute},
		{"last 90 days", host, 90 * 24 * time.Hour, 90 * 24 * time.Hour, "1h", 3 * time.Hour},
		// a short range older than raw retention falls back to rollups
		{"hour two weeks ago", host, time.Hour, 14 * 24 * time.Hour, "1m", time.Minute},
		{"hour two months ago", host, time.Hour, 60 * 24 * time.Hour, "1h", time.Hour},
		{"cores have no rollups", seriesDefs["cpu_core"], 7 * 24 * time.Hour, 7 * 24 * time.Hour, "raw", 605 * time.Second},
	}
	for _, tt := range tests {
		from := now.Add(-tt.ago)
		source, step := pickSource(tt.def, from, from.Add(tt.span), now, defaultMaxPoints, DefaultRetention)
		if source != tt.wantSource || step != tt.wantStep {
			t.Errorf("%s: got %s %v, want %s %v", tt.name, source, step, tt.wantSource, tt.wantStep)
		}
		if points := tt.span / step; points > defaultMaxPoints {
			t.Errorf("%s: %d points is over the limit", tt.name, points)
		}
	}
}

func TestSeriesSQL(t *testing.T) {
	from := time.Unix(1700000000, 0)
	q := SeriesQuery{Metric: "disk_used", Label: "/", From: from, To: from.Add(time.Hour), Max: true}
	sql, args := seriesSQL(seriesDefs["disk_used"], "1m", time.Minute, 7, q)

	for _, want := range []string{"FROM disk_metrics_1m", "max(used_pct_max)", "time_bucket($1::interval, bucket)", "AND mount::text = $5"} {
		if !strings.Contains(sql, want) {
			t.Errorf("expected %q in sql:\n%s", want, sql)
		}
	}
	if len(args) != 5 || args[0] != "60 seconds" || args[1] != int64(7) || args[4] != "/" {
		t.Errorf("unexpected args: %v", args)
	}
}
