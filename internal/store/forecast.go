package store

import (
	"context"
	"fmt"
	"time"
)

// A disk is forecast to fill up only when its usage has grown steadily over
// the window. Disks that jump up and down, like ones with rotating logs,
// have a low r2 and get no forecast.
const (
	forecastWindow     = 7 * 24 * time.Hour
	forecastMinSamples = 24
	forecastMinR2      = 0.6
	// forecasts further out than this are not worth showing, and alert
	// rules see this many days for a disk that is not filling up
	forecastHorizonDays = 365
)

// DiskForecast is a disk's growth over the forecast window and when it
// fills up at that rate
type DiskForecast struct {
	Host        string
	Device      string
	Mount       string
	UsedPct     float64
	PctPerDay   float64
	BytesPerDay float64
	// DaysToFull is nil when the disk is not filling up
	DaysToFull *float64
	// Samples is the number of hourly values behind the forecast
	Samples int
}

// forecastDays returns how many days are left until a disk is full, and
// false when the disk is not filling up or there is too little history to say
func forecastDays(usedPct float64, pctPerDay float64, r2 float64, samples int) (float64, bool) {
	if samples < forecastMinSamples || pctPerDay <= 0 || r2 < forecastMinR2 {
		return 0, false
	}
	days := (100 - usedPct) / pctPerDay
	if days > forecastHorizonDays {
		return 0, false
	}
	return max(days, 0), true
}

// DiskForecasts returns a forecast for each of a host's disks
func (s *Store) DiskForecasts(ctx context.Context, host string) ([]DiskForecast, error) {
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return nil, err
	}
	return s.queryForecasts(ctx, "AND d.host_id = $2", hostID)
}

// SoonestDiskFull returns, for each host with a disk that is filling up,
// the days until the first one is full
func (s *Store) SoonestDiskFull(ctx context.Context) (map[string]float64, error) {
	forecasts, err := s.queryForecasts(ctx, "")
	if err != nil {
		return nil, err
	}
	soonest := map[string]float64{}
	for _, forecast := range forecasts {
		if forecast.DaysToFull == nil {
			continue
		}
		if days, ok := soonest[forecast.Host]; !ok || *forecast.DaysToFull < days {
			soonest[forecast.Host] = *forecast.DaysToFull
		}
	}
	return soonest, nil
}

// DaysUntilFull is the value disk forecast alert rules check. A disk that
// is not filling up reads as forecastHorizonDays, so an open alert on it
// resolves. It returns ErrNotFound while there is too little history.
func (s *Store) DaysUntilFull(ctx context.Context, host string, device string) (float64, error) {
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return 0, err
	}
	forecasts, err := s.queryForecasts(ctx, "AND d.host_id = $2 AND d.device = $3", hostID, device)
	if err != nil {
		return 0, err
	}
	// a device mounted twice has the same data under both mounts
	days := float64(forecastHorizonDays)
	found := false
	for _, forecast := range forecasts {
		if forecast.Samples < forecastMinSamples {
			continue
		}
		found = true
		if forecast.DaysToFull != nil {
			days = min(days, *forecast.DaysToFull)
		}
	}
	if !found {
		return 0, ErrNotFound
	}
	return days, nil
}

// queryForecasts fits a line to each disk's hourly usage over the window.
// filter is a constant condition on d.host_id or d.device using $2 and up.
func (s *Store) queryForecasts(ctx context.Context, filter string, args ...any) ([]DiskForecast, error) {
	start := time.Now().Add(-forecastWindow)
	// x is days since the start of the window, so the slopes are per day
	sql := fmt.Sprintf(`
		SELECT h.name, d.device, d.mount,
		       last(d.used_pct, d.bucket),
		       regr_slope(d.used_pct, extract(epoch FROM d.bucket - $1::timestamptz) / 86400),
		       regr_slope(d.used_bytes, extract(epoch FROM d.bucket - $1::timestamptz) / 86400),
		       regr_r2(d.used_pct, extract(epoch FROM d.bucket - $1::timestamptz) / 86400),
		       count(d.used_pct)
		FROM disk_metrics_1h d
		JOIN hosts h ON h.id = d.host_id
		WHERE d.bucket >= $1 %s
		GROUP BY h.name, d.device, d.mount
		ORDER BY h.name, d.mount`, filter)
	rows, err := s.pool.Query(ctx, sql, append([]any{start}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	forecasts := []DiskForecast{}
	for rows.Next() {
		var forecast DiskForecast
		var usedPct, pctPerDay, bytesPerDay, r2 *float64
		if err := rows.Scan(&forecast.Host, &forecast.Device, &forecast.Mount, &usedPct, &pctPerDay, &bytesPerDay, &r2, &forecast.Samples); err != nil {
			return nil, err
		}
		// nulls come from a single sample or no usable values
		forecast.UsedPct = valueOrZero(usedPct)
		forecast.PctPerDay = valueOrZero(pctPerDay)
		forecast.BytesPerDay = valueOrZero(bytesPerDay)
		if days, ok := forecastDays(forecast.UsedPct, forecast.PctPerDay, valueOrZero(r2), forecast.Samples); ok {
			forecast.DaysToFull = &days
		}
		forecasts = append(forecasts, forecast)
	}
	return forecasts, rows.Err()
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
