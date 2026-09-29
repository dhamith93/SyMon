package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// EndpointCheck is one HTTP check the collector ran for an endpoint rule
type EndpointCheck struct {
	Time time.Time
	// Name is the rule's name
	Name   string
	URL    string
	Method string
	// StatusCode is 0 when there was no response
	StatusCode int
	// OK is true when the response had the expected status code
	OK bool
	// Latency is the time until the response headers arrived, 0 when there
	// was no response
	Latency time.Duration
	Error   string
	// CertExpires is when the server's certificate expires, zero without one
	CertExpires time.Time
}

func (s *Store) SaveEndpointCheck(ctx context.Context, check EndpointCheck) error {
	var statusCode *int
	var latencyMs *float64
	var certExpires *time.Time
	if check.StatusCode != 0 {
		ms := float64(check.Latency) / float64(time.Millisecond)
		statusCode, latencyMs = &check.StatusCode, &ms
	}
	if !check.CertExpires.IsZero() {
		certExpires = &check.CertExpires
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO endpoint_checks (time, name, url, method, status_code, ok, latency_ms, error, cert_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		check.Time, check.Name, check.URL, check.Method, statusCode, check.OK, latencyMs, check.Error, certExpires)
	return err
}

const endpointCheckColumns = "time, name, url, method, status_code, ok, latency_ms, error, cert_expires_at"

func scanEndpointCheck(row pgx.Row) (EndpointCheck, error) {
	var check EndpointCheck
	var statusCode *int
	var latencyMs *float64
	var certExpires *time.Time
	err := row.Scan(&check.Time, &check.Name, &check.URL, &check.Method, &statusCode, &check.OK, &latencyMs, &check.Error, &certExpires)
	if certExpires != nil {
		check.CertExpires = *certExpires
	}
	if statusCode != nil {
		check.StatusCode = *statusCode
	}
	if latencyMs != nil {
		check.Latency = time.Duration(*latencyMs * float64(time.Millisecond))
	}
	return check, err
}

// LatestEndpointCheck returns the newest check of an endpoint rule
func (s *Store) LatestEndpointCheck(ctx context.Context, name string) (EndpointCheck, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+endpointCheckColumns+` FROM endpoint_checks
		WHERE name = $1 ORDER BY time DESC LIMIT 1`, name)
	check, err := scanEndpointCheck(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return EndpointCheck{}, ErrNotFound
	}
	return check, err
}

// LatestCertificate returns when the certificate seen by an endpoint rule's
// newest check with one expires, and when that check ran. A failed check
// has no certificate, so the one before it counts.
func (s *Store) LatestCertificate(ctx context.Context, name string) (time.Time, time.Time, error) {
	var expires, checked time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT cert_expires_at, time FROM endpoint_checks
		WHERE name = $1 AND cert_expires_at IS NOT NULL
		ORDER BY time DESC LIMIT 1`, name).Scan(&expires, &checked)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, time.Time{}, ErrNotFound
	}
	return expires, checked, err
}

// EndpointSummary is an endpoint's newest check up to the end of a range,
// and how it did over the range
type EndpointSummary struct {
	Latest EndpointCheck
	Checks int
	// UptimePct is the share of checks that were OK
	UptimePct float64
	// AvgLatency is over the checks that got a response
	AvgLatency time.Duration
	// CertExpires is from the newest check that saw a certificate, zero
	// without one
	CertExpires time.Time
}

// Endpoints summarizes every endpoint checked within the range
func (s *Store) Endpoints(ctx context.Context, from time.Time, to time.Time) ([]EndpointSummary, error) {
	if !to.After(from) {
		return nil, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	rows, err := s.pool.Query(ctx, `
		WITH ranged AS (
			SELECT name, count(*) AS checks, avg(ok::int) * 100 AS uptime, avg(latency_ms) AS latency
			FROM endpoint_checks
			WHERE time >= $1 AND time < $2
			GROUP BY name
		)
		SELECT r.checks, r.uptime, r.latency, l.time, l.name, l.url, l.method, l.status_code, l.ok, l.latency_ms, l.error,
		       (SELECT cert_expires_at FROM endpoint_checks
		        WHERE name = r.name AND time < $2 AND cert_expires_at IS NOT NULL
		        ORDER BY time DESC LIMIT 1)
		FROM ranged r
		CROSS JOIN LATERAL (
			SELECT `+endpointCheckColumns+` FROM endpoint_checks
			WHERE name = r.name AND time < $2
			ORDER BY time DESC LIMIT 1
		) l
		ORDER BY r.name`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := []EndpointSummary{}
	for rows.Next() {
		var summary EndpointSummary
		var latencyMs *float64
		var statusCode *int
		var lastLatencyMs *float64
		var certExpires *time.Time
		latest := &summary.Latest
		if err := rows.Scan(&summary.Checks, &summary.UptimePct, &latencyMs, &latest.Time, &latest.Name, &latest.URL, &latest.Method,
			&statusCode, &latest.OK, &lastLatencyMs, &latest.Error, &certExpires); err != nil {
			return nil, err
		}
		if certExpires != nil {
			summary.CertExpires = *certExpires
		}
		if latencyMs != nil {
			summary.AvgLatency = time.Duration(*latencyMs * float64(time.Millisecond))
		}
		if statusCode != nil {
			latest.StatusCode = *statusCode
		}
		if lastLatencyMs != nil {
			latest.Latency = time.Duration(*lastLatencyMs * float64(time.Millisecond))
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// endpointSeriesColumns are the metrics EndpointSeries charts
var endpointSeriesColumns = map[string]string{
	// milliseconds until the response headers, over checks that got one
	"latency": "avg(latency_ms)",
	// share of checks that were OK
	"availability": "avg(ok::int) * 100",
}

// EndpointSeries returns an endpoint's latency or availability over a range,
// with at most maxPoints buckets
func (s *Store) EndpointSeries(ctx context.Context, name string, metric string, from time.Time, to time.Time, maxPoints int) (SeriesResult, error) {
	column, ok := endpointSeriesColumns[metric]
	if !ok {
		return SeriesResult{}, fmt.Errorf("%w: unknown endpoint metric %q", ErrInvalid, metric)
	}
	if !to.After(from) {
		return SeriesResult{}, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	if maxPoints <= 0 {
		maxPoints = defaultMaxPoints
	}
	// rounded like pickSource does for host metrics
	step := (to.Sub(from) / time.Duration(maxPoints)).Truncate(time.Second) + time.Second

	// column comes from endpointSeriesColumns, never from the caller
	rows, err := s.pool.Query(ctx, `
		SELECT time_bucket($1::interval, time) AS t, `+column+`
		FROM endpoint_checks
		WHERE name = $2 AND time >= $3 AND time < $4
		GROUP BY t ORDER BY t`, fmt.Sprintf("%d seconds", int64(step.Seconds())), name, from, to)
	if err != nil {
		return SeriesResult{}, err
	}
	defer rows.Close()

	result := SeriesResult{Source: "raw", Step: step, Series: []Series{}}
	series := Series{Label: name}
	for rows.Next() {
		var at time.Time
		var value *float64
		if err := rows.Scan(&at, &value); err != nil {
			return SeriesResult{}, err
		}
		// no response in the whole bucket, so no latency
		if value == nil {
			continue
		}
		series.Points = append(series.Points, Point{Time: at, Value: *value})
	}
	if len(series.Points) > 0 {
		result.Series = append(result.Series, series)
	}
	return result, rows.Err()
}
