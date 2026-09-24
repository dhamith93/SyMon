-- Per container metrics. name is the runtime's name, or the short ID when
-- the socket was not reachable. Traffic is null for host network containers.

CREATE TABLE container_metrics (
    time         timestamptz NOT NULL,
    host_id      bigint NOT NULL,
    container_id text NOT NULL,
    name         text NOT NULL,
    cpu_cores    double precision,
    cpu_pct      double precision,
    mem_used     double precision,
    mem_pct      double precision,
    rx_bps       double precision,
    tx_bps       double precision,
    read_bps     double precision,
    write_bps    double precision,
    pids         double precision
);
SELECT create_hypertable('container_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON container_metrics (host_id, name, time DESC);

ALTER TABLE container_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, name');
SELECT add_compression_policy('container_metrics', INTERVAL '2 days');

CREATE MATERIALIZED VIEW container_metrics_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id, name,
       avg(cpu_pct)   AS cpu_pct,
       max(cpu_pct)   AS cpu_pct_max,
       avg(mem_used)  AS mem_used,
       max(mem_used)  AS mem_used_max,
       avg(rx_bps)    AS rx_bps,
       max(rx_bps)    AS rx_bps_max,
       avg(tx_bps)    AS tx_bps,
       max(tx_bps)    AS tx_bps_max,
       avg(read_bps)  AS read_bps,
       avg(write_bps) AS write_bps
FROM container_metrics
GROUP BY bucket, host_id, name
WITH NO DATA;

CREATE MATERIALIZED VIEW container_metrics_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id, name,
       avg(cpu_pct)      AS cpu_pct,
       max(cpu_pct_max)  AS cpu_pct_max,
       avg(mem_used)     AS mem_used,
       max(mem_used_max) AS mem_used_max,
       avg(rx_bps)       AS rx_bps,
       max(rx_bps_max)   AS rx_bps_max,
       avg(tx_bps)       AS tx_bps,
       max(tx_bps_max)   AS tx_bps_max,
       avg(read_bps)     AS read_bps,
       avg(write_bps)    AS write_bps
FROM container_metrics_1m
GROUP BY 1, host_id, name
WITH NO DATA;

SELECT add_continuous_aggregate_policy('container_metrics_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('container_metrics_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
