package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/kube"
	"github.com/jackc/pgx/v5"
)

// ErrClusterTaken is a report from a host other than the one that reports
// the cluster
var ErrClusterTaken = errors.New("another host reports this cluster, remove the cluster on the dashboard to move its reader")

// Cluster is a Kubernetes cluster with the newest state that was read
type Cluster struct {
	Name string
	// Host is the host whose agent reads it, empty once that host is removed
	Host string
	// ReportedAt is the last report, Error why it could not read the API
	ReportedAt time.Time
	Error      string
	State      kube.ClusterState
}

// Stale is true when the state is too old to alert on, like when the
// reader stopped or cannot reach the API
func (c Cluster) Stale(now time.Time) bool {
	return now.Sub(time.Unix(c.State.Time, 0)) > kube.StaleAfter
}

// KubeRestart is a container restart the collector saw between two reports
type KubeRestart struct {
	Time time.Time
	kube.Restart
}

// SaveClusterReport keeps what a host's agent read of a cluster. The first
// report makes the cluster. A report that could not read the API only
// records the error, so the last state stays and restarts are counted
// against it once the API answers again.
func (s *Store) SaveClusterReport(ctx context.Context, host string, state kube.ClusterState) error {
	if strings.TrimSpace(state.Cluster) == "" || len(state.Cluster) > 100 {
		return fmt.Errorf("%w: a cluster needs a name of up to 100 characters", ErrInvalid)
	}
	hostID, err := s.hostID(ctx, host)
	if err != nil {
		return err
	}
	at := time.Unix(state.Time, 0)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var clusterID int64
	var owner *int64
	var prev kube.ClusterState
	err = tx.QueryRow(ctx, `SELECT id, host_id, state FROM kube_clusters WHERE name = $1 FOR UPDATE`, state.Cluster).Scan(&clusterID, &owner, &prev)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		saved := state
		if state.Error != "" {
			// nothing read yet
			saved = kube.ClusterState{Cluster: state.Cluster}
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO kube_clusters (name, host_id, state, error, reported_at) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (name) DO NOTHING RETURNING id`, state.Cluster, hostID, saved, state.Error, at).Scan(&clusterID)
		if errors.Is(err, pgx.ErrNoRows) {
			// another report made it first
			return ErrClusterTaken
		}
		if err != nil {
			return err
		}
		if state.Error == "" {
			if err := saveClusterMetrics(ctx, tx, clusterID, state, nil); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	case err != nil:
		return err
	case owner != nil && *owner != hostID:
		return ErrClusterTaken
	}

	if state.Error != "" {
		if _, err := tx.Exec(ctx, `UPDATE kube_clusters SET host_id = $2, error = $3, reported_at = $4 WHERE id = $1`, clusterID, hostID, state.Error, at); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if state.Time <= prev.Time {
		// an older report arriving late
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `UPDATE kube_clusters SET host_id = $2, state = $3, error = '', reported_at = $4 WHERE id = $1`, clusterID, hostID, state, at); err != nil {
		return err
	}
	if err := saveClusterMetrics(ctx, tx, clusterID, state, kube.Restarts(prev, state)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func saveClusterMetrics(ctx context.Context, tx pgx.Tx, clusterID int64, state kube.ClusterState, restarts []kube.Restart) error {
	at := time.Unix(state.Time, 0)
	counts := state.Counts()
	batch := &pgx.Batch{}
	total := 0
	for _, r := range restarts {
		total += r.Count
		batch.Queue(`INSERT INTO kube_restarts (time, cluster_id, namespace, workload, pod, container, count, reason)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, at, clusterID, r.Namespace, r.Workload, r.Pod, r.Container, r.Count, r.Reason)
	}
	batch.Queue(`INSERT INTO kube_metrics (time, cluster_id, nodes, nodes_ready, pods, pods_running, pods_pending, pods_not_ready, pods_failed, workloads, workloads_ready, restarts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		at, clusterID, counts.Nodes, counts.NodesReady, counts.Pods, counts.PodsRunning, counts.PodsPending,
		counts.PodsNotReady, counts.PodsFailed, counts.Workloads, counts.WorkloadsReady, total)
	return tx.SendBatch(ctx, batch).Close()
}

const clusterColumns = `c.name, coalesce(h.name, ''), c.reported_at, c.error, c.state
	FROM kube_clusters c LEFT JOIN hosts h ON h.id = c.host_id`

func scanCluster(row pgx.Row) (Cluster, error) {
	var c Cluster
	err := row.Scan(&c.Name, &c.Host, &c.ReportedAt, &c.Error, &c.State)
	return c, err
}

// Clusters lists the clusters by name
func (s *Store) Clusters(ctx context.Context) ([]Cluster, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+clusterColumns+` ORDER BY c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	clusters := []Cluster{}
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, err
		}
		clusters = append(clusters, c)
	}
	return clusters, rows.Err()
}

func (s *Store) Cluster(ctx context.Context, name string) (Cluster, error) {
	c, err := scanCluster(s.pool.QueryRow(ctx, `SELECT `+clusterColumns+` WHERE c.name = $1`, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return Cluster{}, ErrNotFound
	}
	return c, err
}

// RemoveCluster forgets a cluster, so another host may report it. Its
// charts and restarts stay until retention drops them.
func (s *Store) RemoveCluster(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM kube_clusters WHERE name = $1`, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecentRestarts are a cluster's restarts since a time, newest first, at
// most limit of them
func (s *Store) RecentRestarts(ctx context.Context, cluster string, since time.Time, limit int) ([]KubeRestart, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.time, r.namespace, r.workload, r.pod, r.container, r.count, r.reason
		FROM kube_restarts r JOIN kube_clusters c ON c.id = r.cluster_id
		WHERE c.name = $1 AND r.time >= $2
		ORDER BY r.time DESC LIMIT $3`, cluster, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	restarts := []KubeRestart{}
	for rows.Next() {
		var r KubeRestart
		if err := rows.Scan(&r.Time, &r.Namespace, &r.Workload, &r.Pod, &r.Container, &r.Count, &r.Reason); err != nil {
			return nil, err
		}
		restarts = append(restarts, r)
	}
	return restarts, rows.Err()
}

// RestartsSince adds up a cluster's restarts since a time by workload, as
// namespace/kind/name
func (s *Store) RestartsSince(ctx context.Context, cluster string, since time.Time) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.namespace || '/' || r.workload, sum(r.count)
		FROM kube_restarts r JOIN kube_clusters c ON c.id = r.cluster_id
		WHERE c.name = $1 AND r.time >= $2
		GROUP BY 1`, cluster, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	totals := map[string]int{}
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		totals[key] = count
	}
	return totals, rows.Err()
}

// clusterSeries are the charts of a cluster, each with its lines. Columns
// come from here, never from the caller.
var clusterSeries = map[string][]struct{ label, column string }{
	"nodes": {
		{"ready", "avg(nodes_ready)::float8"},
		{"not ready", "avg(nodes - nodes_ready)::float8"},
	},
	"pods": {
		{"running", "avg(pods_running)::float8"},
		{"pending", "avg(pods_pending)::float8"},
		{"not ready", "avg(pods_not_ready)::float8"},
		{"failed", "avg(pods_failed)::float8"},
	},
	"workloads": {
		{"ready", "avg(workloads_ready)::float8"},
		{"not ready", "avg(workloads - workloads_ready)::float8"},
	},
	"restarts": {
		{"restarts", "sum(restarts)::float8"},
	},
}

// ClusterSeries returns one of a cluster's charts over a range, with at most
// maxPoints buckets
func (s *Store) ClusterSeries(ctx context.Context, cluster string, metric string, from time.Time, to time.Time, maxPoints int) (SeriesResult, error) {
	lines, ok := clusterSeries[metric]
	if !ok {
		return SeriesResult{}, fmt.Errorf("%w: unknown cluster metric %q", ErrInvalid, metric)
	}
	if !to.After(from) {
		return SeriesResult{}, fmt.Errorf("%w: from must be before to", ErrInvalid)
	}
	if maxPoints <= 0 {
		maxPoints = defaultMaxPoints
	}
	// rounded like pickSource does for host metrics
	step := (to.Sub(from) / time.Duration(maxPoints)).Truncate(time.Second) + time.Second

	columns := make([]string, len(lines))
	for i, line := range lines {
		columns[i] = line.column
	}
	rows, err := s.pool.Query(ctx, `
		SELECT time_bucket($1::interval, m.time) AS t, `+strings.Join(columns, ", ")+`
		FROM kube_metrics m JOIN kube_clusters c ON c.id = m.cluster_id
		WHERE c.name = $2 AND m.time >= $3 AND m.time < $4
		GROUP BY t ORDER BY t`, fmt.Sprintf("%d seconds", int64(step.Seconds())), cluster, from, to)
	if err != nil {
		return SeriesResult{}, err
	}
	defer rows.Close()

	series := make([]Series, len(lines))
	for i, line := range lines {
		series[i].Label = line.label
	}
	for rows.Next() {
		var at time.Time
		values := make([]float64, len(lines))
		targets := []any{&at}
		for i := range values {
			targets = append(targets, &values[i])
		}
		if err := rows.Scan(targets...); err != nil {
			return SeriesResult{}, err
		}
		for i, value := range values {
			series[i].Points = append(series[i].Points, Point{Time: at, Value: value})
		}
	}
	return SeriesResult{Source: "raw", Step: step, Series: series}, rows.Err()
}
