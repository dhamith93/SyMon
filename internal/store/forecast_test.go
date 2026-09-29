package store

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestForecastDays(t *testing.T) {
	// 600 GB used at 60% leaves 400 GB, the rest is reserved or already used
	const gb = 1e9
	tests := []struct {
		name        string
		usedPct     float64
		usedBytes   float64
		bytesPerDay float64
		r2          float64
		samples     int
		wantDays    float64
		wantOK      bool
	}{
		{"steady growth", 60, 600 * gb, 20 * gb, 0.95, 168, 20, true},
		{"flat", 60, 600 * gb, 0, 1, 168, 0, false},
		{"shrinking", 60, 600 * gb, -10 * gb, 0.9, 168, 0, false},
		{"noisy", 60, 600 * gb, 20 * gb, 0.3, 168, 0, false},
		{"one day of history", 60, 600 * gb, 20 * gb, 0.95, 24, 20, true},
		{"too little history", 60, 600 * gb, 20 * gb, 0.95, 23, 0, false},
		{"beyond the horizon", 60, 600 * gb, 1 * gb, 0.95, 168, 0, false},
		{"already full", 100, 600 * gb, 1 * gb, 0.95, 168, 0, true},
		{"empty disk", 0, 0, 1 * gb, 0.95, 168, 0, false},
	}
	for _, tt := range tests {
		days, ok := forecastDays(tt.usedPct, tt.usedBytes, tt.bytesPerDay, tt.r2, tt.samples)
		if ok != tt.wantOK || days != tt.wantDays {
			t.Errorf("%s: got %v %v, want %v %v", tt.name, days, ok, tt.wantDays, tt.wantOK)
		}
	}
}

// refreshRollups materializes rollups by hand, so the test does not depend
// on when the scheduler last ran them. A scheduled refresh of the same
// rollup may still be running, so it retries.
func refreshRollups(t *testing.T, st *Store, views ...string) {
	t.Helper()
	for _, view := range views {
		for attempt := 1; ; attempt++ {
			_, err := st.pool.Exec(context.Background(), "CALL refresh_continuous_aggregate('"+view+"', NULL, NULL)")
			if err == nil {
				break
			}
			if attempt == 20 {
				t.Fatal(err)
			}
			time.Sleep(250 * time.Millisecond)
		}
	}
}

func TestDiskForecasts(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	hostID, err := st.hostID(ctx, "web1")
	if err != nil {
		t.Fatal(err)
	}

	// two days of samples: /data grows 2% a day and reaches 60% now, so it
	// is full in about 20 days. / stays at 40%. Percents are whole numbers,
	// like the agent sends them.
	now := time.Now()
	batch := &pgx.Batch{}
	insert := `INSERT INTO disk_metrics (time, host_id, device, mount, fstype, size_bytes, used_bytes, used_pct, inodes_used_pct)
		VALUES ($1, $2, $3, $4, 'ext4', 1e9, $5, $6, 1)`
	for at := now.Add(-48 * time.Hour); at.Before(now); at = at.Add(10 * time.Minute) {
		used := 60 + 2*at.Sub(now).Hours()/24
		batch.Queue(insert, at, hostID, "/dev/sdb1", "/data", used*1e7, math.Round(used))
		batch.Queue(insert, at, hostID, "/dev/sda1", "/", 40e7, 40.0)
	}
	if err := st.sendBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	refreshRollups(t, st, "disk_metrics_1m", "disk_metrics_1h")

	forecasts, err := st.DiskForecasts(ctx, "web1")
	if err != nil {
		t.Fatal(err)
	}
	if len(forecasts) != 2 {
		t.Fatalf("expected 2 disks, got %+v", forecasts)
	}
	root, data := forecasts[0], forecasts[1]
	if root.Mount != "/" || root.DaysToFull != nil || root.PctPerDay != 0 {
		t.Errorf("expected / not to be filling up, got %+v", root)
	}
	if data.Mount != "/data" || data.DaysToFull == nil || math.Abs(*data.DaysToFull-20) > 2 {
		t.Errorf("expected /data full in about 20 days, got %+v", data)
	}
	if math.Abs(data.PctPerDay-2) > 0.01 || math.Abs(data.BytesPerDay-2e7) > 1e5 {
		t.Errorf("expected 2%% and 20 MB a day, got %v and %v", data.PctPerDay, data.BytesPerDay)
	}

	soonest, err := st.SoonestDiskFull(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(soonest) != 1 || math.Abs(soonest["web1"]-*data.DaysToFull) > 1e-9 {
		t.Errorf("expected web1 to fill up in %v days, got %v", *data.DaysToFull, soonest)
	}

	days, err := st.DaysUntilFull(ctx, "web1", "/dev/sdb1")
	if err != nil || math.Abs(days-*data.DaysToFull) > 1e-9 {
		t.Errorf("expected %v days for /dev/sdb1, got %v %v", *data.DaysToFull, days, err)
	}
	days, err = st.DaysUntilFull(ctx, "web1", "/dev/sda1")
	if err != nil || days != forecastHorizonDays {
		t.Errorf("expected the horizon for a disk that is not filling up, got %v %v", days, err)
	}
	if _, err := st.DaysUntilFull(ctx, "web1", "/dev/sdz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown disk, got %v", err)
	}
}
