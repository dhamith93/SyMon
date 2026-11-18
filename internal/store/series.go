package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// seriesDef says where a chartable metric lives. Rollups of table are
// table_1m and table_1h, with the same column names.
type seriesDef struct {
	table  string
	column string
	// label splits the metric into one series per disk, interface and so
	// on. Empty means one series per host.
	label string
	// hasMax is true when the rollups also keep column_max
	hasMax bool
	// rawOnly metrics have no rollups
	rawOnly bool
	// perMinute columns count events over the table's seconds column, at
	// every resolution, and read as events per minute
	perMinute bool
	// perSecond columns count bytes, and read as bytes per second over the
	// bucket, or over the table's seconds column when a row covers more
	perSecond bool
	// topLabels keeps that many labels, the most bytes over the range
	// first, and adds the others and the empty label into otherLabel
	topLabels int
}

// otherLabel is the series for the labels past topLabels. It is longer
// than a kernel comm can be, so no program can have the same name.
const otherLabel = "All other programs"

var seriesDefs = map[string]seriesDef{
	"cpu":                {table: "host_metrics", column: "cpu_pct", hasMax: true},
	"cpu_core":           {table: "cpu_core_metrics", column: "pct", label: "core", rawOnly: true},
	"load1":              {table: "host_metrics", column: "load1"},
	"load5":              {table: "host_metrics", column: "load5"},
	"load15":             {table: "host_metrics", column: "load15"},
	"memory":             {table: "host_metrics", column: "mem_used_pct", hasMax: true},
	"memory_used":        {table: "host_metrics", column: "mem_used_mib"},
	"swap":               {table: "host_metrics", column: "swap_used_pct", hasMax: true},
	"swap_used":          {table: "host_metrics", column: "swap_used_mib"},
	"psi_cpu":            {table: "host_metrics", column: "psi_cpu_some"},
	"psi_memory":         {table: "host_metrics", column: "psi_memory_some"},
	"psi_memory_full":    {table: "host_metrics", column: "psi_memory_full"},
	"psi_io":             {table: "host_metrics", column: "psi_io_some"},
	"psi_io_full":        {table: "host_metrics", column: "psi_io_full"},
	"tcp_established":    {table: "host_metrics", column: "tcp_established"},
	"tcp_time_wait":      {table: "host_metrics", column: "tcp_time_wait"},
	"tcp_close_wait":     {table: "host_metrics", column: "tcp_close_wait"},
	"tcp_listen":         {table: "host_metrics", column: "tcp_listen"},
	"tcp_total":          {table: "host_metrics", column: "tcp_total"},
	"disk_used":          {table: "disk_metrics", column: "used_pct", label: "mount", hasMax: true},
	"disk_inodes":        {table: "disk_metrics", column: "inodes_used_pct", label: "mount"},
	"disk_read":          {table: "disk_io_metrics", column: "read_bps", label: "device", hasMax: true},
	"disk_write":         {table: "disk_io_metrics", column: "write_bps", label: "device", hasMax: true},
	"disk_util":          {table: "disk_io_metrics", column: "util_pct", label: "device", hasMax: true},
	"net_rx":             {table: "net_metrics", column: "rx_bps", label: "iface", hasMax: true},
	"net_tx":             {table: "net_metrics", column: "tx_bps", label: "iface", hasMax: true},
	"temperature":        {table: "temp_metrics", column: "celsius", label: "sensor", hasMax: true},
	"custom":             {table: "custom_metrics", column: "value", label: "name", hasMax: true},
	"container_cpu":      {table: "container_metrics", column: "cpu_pct", label: "name", hasMax: true},
	"container_memory":   {table: "container_metrics", column: "mem_used", label: "name", hasMax: true},
	"container_rx":       {table: "container_metrics", column: "rx_bps", label: "name", hasMax: true},
	"container_tx":       {table: "container_metrics", column: "tx_bps", label: "name", hasMax: true},
	"container_io_read":  {table: "container_metrics", column: "read_bps", label: "name"},
	"container_io_write": {table: "container_metrics", column: "write_bps", label: "name"},
	"proc_started":       {table: "process_activity", column: "execs", perMinute: true},
	"proc_ended":         {table: "process_activity", column: "exits", perMinute: true},
	"proc_failed":        {table: "process_activity", column: "nonzero_exits", perMinute: true},
	"proc_signaled":      {table: "process_activity", column: "signaled", perMinute: true},
	"proc_failed_execs":  {table: "process_activity", column: "failed_execs", perMinute: true},
	"proc_net_rx":        {table: "process_network", column: "rx_bytes", label: "name", perSecond: true, topLabels: 7},
	"proc_net_tx":        {table: "process_network", column: "tx_bytes", label: "name", perSecond: true, topLabels: 7},
}

// SeriesMetrics lists the metric names QuerySeries accepts
func SeriesMetrics() []string {
	names := make([]string, 0, len(seriesDefs))
	for name := range seriesDefs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type SeriesQuery struct {
	Host   string
	Metric string
	// Label keeps only one series, for example a mount point or a custom
	// metric name. Empty returns all of them.
	Label     string
	From      time.Time
	To        time.Time
	MaxPoints int
	// Max returns the peak of each bucket instead of the average
	Max bool
}

type Point struct {
	Time  time.Time
	Value float64
}

type Series struct {
	Label  string
	Points []Point
}

type SeriesResult struct {
	// Source is "raw", "1m" or "1h"
	Source string
	Step   time.Duration
	Series []Series
}

const defaultMaxPoints = 1000

// QuerySeries returns a metric over a time range with at most MaxPoints
// points per series, read from the coarsest data that still gives that many
func (s *Store) QuerySeries(ctx context.Context, q SeriesQuery) (SeriesResult, error) {
	def, ok := seriesDefs[q.Metric]
	if !ok {
		return SeriesResult{}, fmt.Errorf("%w: unknown metric %q", ErrInvalid, q.Metric)
	}
	if !q.To.After(q.From) {
		return SeriesResult{}, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	if q.Max && !def.hasMax {
		return SeriesResult{}, fmt.Errorf("%w: metric %q has no max", ErrInvalid, q.Metric)
	}
	if q.MaxPoints <= 0 {
		q.MaxPoints = defaultMaxPoints
	}
	hostID, err := s.hostID(ctx, q.Host)
	if err != nil {
		return SeriesResult{}, err
	}

	source, step := pickSource(def, q.From, q.To, time.Now(), q.MaxPoints, s.retention)
	sql, args := seriesSQL(def, source, step, hostID, q)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return SeriesResult{}, err
	}
	defer rows.Close()

	series, err := readSeries(rows)
	if err != nil {
		return SeriesResult{}, err
	}
	return SeriesResult{Source: source, Step: step, Series: series}, nil
}

// readSeries reads rows of time, label and value, ordered by label, into
// one series per label
func readSeries(rows pgx.Rows) ([]Series, error) {
	series := []Series{}
	for rows.Next() {
		var at time.Time
		var label string
		var value *float64
		if err := rows.Scan(&at, &label, &value); err != nil {
			return nil, err
		}
		if value == nil {
			continue
		}
		// rows are ordered by label, so a new label starts a new series
		if len(series) == 0 || series[len(series)-1].Label != label {
			series = append(series, Series{Label: label})
		}
		last := &series[len(series)-1]
		last.Points = append(last.Points, Point{Time: at, Value: *value})
	}
	return series, rows.Err()
}

// pickSource chooses the finest data that is still kept for the whole
// range, and a bucket size that gives at most maxPoints buckets
func pickSource(def seriesDef, from time.Time, to time.Time, now time.Time, maxPoints int, retention Retention) (string, time.Duration) {
	step := to.Sub(from) / time.Duration(maxPoints)
	step = step.Truncate(time.Second) + time.Second

	if def.rawOnly || (step < time.Minute && !from.Before(now.Add(-retention.Raw))) {
		return "raw", step
	}
	if step < time.Hour && !from.Before(now.Add(-retention.Minute)) {
		return "1m", roundUp(step, time.Minute)
	}
	return "1h", roundUp(step, time.Hour)
}

// topLabelsSQL works out each label's values first and then adds the ones
// past def.topLabels into otherLabel, so a rate is never divided by the
// seconds of several labels at once
func topLabelsSQL(def seriesDef, timeColumn string, label string, value string, table string, filter string) string {
	return fmt.Sprintf(`
		WITH per_label AS (
			SELECT time_bucket($1::interval, %[1]s) AS t, %[2]s AS label, %[3]s AS v, sum(%[6]s) AS total
			FROM %[4]s
			WHERE host_id = $2 AND %[1]s >= $3 AND %[1]s < $4 %[5]s
			GROUP BY t, label
		),
		ranked AS (
			SELECT label, row_number() OVER (ORDER BY sum(total) DESC, label) AS rank
			FROM per_label
			WHERE label <> ''
			GROUP BY label
		),
		folded AS (
			SELECT p.t, CASE WHEN r.rank <= %[7]d THEN p.label ELSE '%[8]s' END AS label, p.v
			FROM per_label p LEFT JOIN ranked r ON r.label = p.label
		)
		SELECT t, label, sum(v) AS v
		FROM folded
		GROUP BY t, label
		ORDER BY label = '%[8]s', label, t`, timeColumn, label, value, table, filter, def.column, def.topLabels, otherLabel)
}

func roundUp(d time.Duration, unit time.Duration) time.Duration {
	if d <= unit {
		return unit
	}
	return ((d + unit - 1) / unit) * unit
}

func seriesSQL(def seriesDef, source string, step time.Duration, hostID int64, q SeriesQuery) (string, []any) {
	table, timeColumn, value := def.table, "time", "avg("+def.column+")"
	if source != "raw" {
		table, timeColumn = def.table+"_"+source, "bucket"
	}
	if def.perMinute {
		value = "sum(" + def.column + ")::float8 * 60 / nullif(sum(seconds)::float8, 0)"
	}
	if def.perSecond {
		// a raw row covers an agent interval, longer than a short bucket
		value = "sum(" + def.column + ")::float8 / greatest(extract(epoch FROM $1::interval)::float8, sum(seconds)::float8)"
	}
	if q.Max {
		value = "max(" + def.column + ")"
		if source != "raw" {
			value = "max(" + def.column + "_max)"
		}
	}
	label := "''"
	if def.label != "" {
		label = def.label + "::text"
	}

	args := []any{fmt.Sprintf("%d seconds", int64(step.Seconds())), hostID, q.From, q.To}
	filter := ""
	if q.Label != "" && def.label != "" {
		filter = "AND " + def.label + "::text = $5"
		args = append(args, q.Label)
	}

	// table and column names come from seriesDefs, never from the caller
	if def.topLabels > 0 {
		return topLabelsSQL(def, timeColumn, label, value, table, filter), args
	}
	sql := fmt.Sprintf(`
		SELECT time_bucket($1::interval, %[1]s) AS t, %[2]s AS label, %[3]s AS v
		FROM %[4]s
		WHERE host_id = $2 AND %[1]s >= $3 AND %[1]s < $4 %[5]s
		GROUP BY t, label
		ORDER BY label, t`, timeColumn, label, value, table, filter)
	return sql, args
}
