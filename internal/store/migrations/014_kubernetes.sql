-- Kubernetes clusters, reported by one agent each. state is the newest
-- kube.ClusterState that was read, error why the last report could not
-- read the API (empty when it could). host_id is the host whose agent reads
-- the cluster, another host cannot report it until the cluster is removed
-- or that host is.

CREATE TABLE kube_clusters (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    host_id     bigint REFERENCES hosts (id) ON DELETE SET NULL,
    state       jsonb NOT NULL,
    error       text NOT NULL DEFAULT '',
    reported_at timestamptz NOT NULL
);

-- counts from every report, for the cluster page's charts. A report every
-- 30 seconds is little data, so there are no rollups and it is kept as long
-- as the hourly rollups. restarts are the ones since the report before.
CREATE TABLE kube_metrics (
    time            timestamptz NOT NULL,
    cluster_id      bigint NOT NULL,
    nodes           integer NOT NULL,
    nodes_ready     integer NOT NULL,
    pods            integer NOT NULL,
    pods_running    integer NOT NULL,
    pods_pending    integer NOT NULL,
    pods_not_ready  integer NOT NULL,
    pods_failed     integer NOT NULL,
    workloads       integer NOT NULL,
    workloads_ready integer NOT NULL,
    restarts        integer NOT NULL
);
SELECT create_hypertable('kube_metrics', by_range('time', INTERVAL '7 days'));
CREATE INDEX ON kube_metrics (cluster_id, time DESC);

ALTER TABLE kube_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'cluster_id');
SELECT add_compression_policy('kube_metrics', INTERVAL '2 days');

-- a row for each container restart seen between two reports. workload is
-- kind/name like deployment/web, reason why the run before ended.
CREATE TABLE kube_restarts (
    time       timestamptz NOT NULL,
    cluster_id bigint NOT NULL,
    namespace  text NOT NULL,
    workload   text NOT NULL,
    pod        text NOT NULL,
    container  text NOT NULL,
    count      integer NOT NULL,
    reason     text NOT NULL DEFAULT ''
);
SELECT create_hypertable('kube_restarts', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON kube_restarts (cluster_id, time DESC);
