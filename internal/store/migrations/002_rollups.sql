-- 1 minute rollups from the raw tables, and 1 hour rollups from the 1 minute
-- ones. Columns keep their raw names and hold the average. Columns where
-- spikes matter also get a _max. materialized_only = false adds the newest,
-- not yet materialized data at query time.

CREATE MATERIALIZED VIEW host_metrics_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id,
       avg(cpu_pct)           AS cpu_pct,
       max(cpu_pct)           AS cpu_pct_max,
       avg(load1)             AS load1,
       avg(load5)             AS load5,
       avg(load15)            AS load15,
       avg(mem_used_pct)      AS mem_used_pct,
       max(mem_used_pct)      AS mem_used_pct_max,
       avg(mem_used_mib)      AS mem_used_mib,
       avg(mem_available_mib) AS mem_available_mib,
       avg(swap_used_pct)     AS swap_used_pct,
       max(swap_used_pct)     AS swap_used_pct_max,
       avg(swap_used_mib)     AS swap_used_mib,
       avg(psi_cpu_some)      AS psi_cpu_some,
       avg(psi_memory_some)   AS psi_memory_some,
       avg(psi_memory_full)   AS psi_memory_full,
       avg(psi_io_some)       AS psi_io_some,
       avg(psi_io_full)       AS psi_io_full,
       avg(tcp_established)   AS tcp_established,
       avg(tcp_time_wait)     AS tcp_time_wait,
       avg(tcp_close_wait)    AS tcp_close_wait,
       avg(tcp_listen)        AS tcp_listen,
       avg(tcp_total)         AS tcp_total
FROM host_metrics
GROUP BY bucket, host_id
WITH NO DATA;

CREATE MATERIALIZED VIEW host_metrics_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id,
       avg(cpu_pct)           AS cpu_pct,
       max(cpu_pct_max)       AS cpu_pct_max,
       avg(load1)             AS load1,
       avg(load5)             AS load5,
       avg(load15)            AS load15,
       avg(mem_used_pct)      AS mem_used_pct,
       max(mem_used_pct_max)  AS mem_used_pct_max,
       avg(mem_used_mib)      AS mem_used_mib,
       avg(mem_available_mib) AS mem_available_mib,
       avg(swap_used_pct)     AS swap_used_pct,
       max(swap_used_pct_max) AS swap_used_pct_max,
       avg(swap_used_mib)     AS swap_used_mib,
       avg(psi_cpu_some)      AS psi_cpu_some,
       avg(psi_memory_some)   AS psi_memory_some,
       avg(psi_memory_full)   AS psi_memory_full,
       avg(psi_io_some)       AS psi_io_some,
       avg(psi_io_full)       AS psi_io_full,
       avg(tcp_established)   AS tcp_established,
       avg(tcp_time_wait)     AS tcp_time_wait,
       avg(tcp_close_wait)    AS tcp_close_wait,
       avg(tcp_listen)        AS tcp_listen,
       avg(tcp_total)         AS tcp_total
FROM host_metrics_1m
GROUP BY 1, host_id
WITH NO DATA;

CREATE MATERIALIZED VIEW disk_metrics_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id, device, mount,
       avg(used_pct)        AS used_pct,
       max(used_pct)        AS used_pct_max,
       avg(used_bytes)      AS used_bytes,
       avg(size_bytes)      AS size_bytes,
       avg(inodes_used_pct) AS inodes_used_pct
FROM disk_metrics
GROUP BY bucket, host_id, device, mount
WITH NO DATA;

CREATE MATERIALIZED VIEW disk_metrics_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id, device, mount,
       avg(used_pct)        AS used_pct,
       max(used_pct_max)    AS used_pct_max,
       avg(used_bytes)      AS used_bytes,
       avg(size_bytes)      AS size_bytes,
       avg(inodes_used_pct) AS inodes_used_pct
FROM disk_metrics_1m
GROUP BY 1, host_id, device, mount
WITH NO DATA;

CREATE MATERIALIZED VIEW disk_io_metrics_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id, device,
       avg(read_bps)  AS read_bps,
       max(read_bps)  AS read_bps_max,
       avg(write_bps) AS write_bps,
       max(write_bps) AS write_bps_max,
       avg(reads_ps)  AS reads_ps,
       avg(writes_ps) AS writes_ps,
       avg(util_pct)  AS util_pct,
       max(util_pct)  AS util_pct_max
FROM disk_io_metrics
GROUP BY bucket, host_id, device
WITH NO DATA;

CREATE MATERIALIZED VIEW disk_io_metrics_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id, device,
       avg(read_bps)      AS read_bps,
       max(read_bps_max)  AS read_bps_max,
       avg(write_bps)     AS write_bps,
       max(write_bps_max) AS write_bps_max,
       avg(reads_ps)      AS reads_ps,
       avg(writes_ps)     AS writes_ps,
       avg(util_pct)      AS util_pct,
       max(util_pct_max)  AS util_pct_max
FROM disk_io_metrics_1m
GROUP BY 1, host_id, device
WITH NO DATA;

CREATE MATERIALIZED VIEW net_metrics_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id, iface,
       avg(rx_bps) AS rx_bps,
       max(rx_bps) AS rx_bps_max,
       avg(tx_bps) AS tx_bps,
       max(tx_bps) AS tx_bps_max,
       avg(rx_pps) AS rx_pps,
       avg(tx_pps) AS tx_pps
FROM net_metrics
GROUP BY bucket, host_id, iface
WITH NO DATA;

CREATE MATERIALIZED VIEW net_metrics_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id, iface,
       avg(rx_bps)     AS rx_bps,
       max(rx_bps_max) AS rx_bps_max,
       avg(tx_bps)     AS tx_bps,
       max(tx_bps_max) AS tx_bps_max,
       avg(rx_pps)     AS rx_pps,
       avg(tx_pps)     AS tx_pps
FROM net_metrics_1m
GROUP BY 1, host_id, iface
WITH NO DATA;

CREATE MATERIALIZED VIEW temp_metrics_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id, sensor,
       avg(celsius) AS celsius,
       max(celsius) AS celsius_max
FROM temp_metrics
GROUP BY bucket, host_id, sensor
WITH NO DATA;

CREATE MATERIALIZED VIEW temp_metrics_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id, sensor,
       avg(celsius)     AS celsius,
       max(celsius_max) AS celsius_max
FROM temp_metrics_1m
GROUP BY 1, host_id, sensor
WITH NO DATA;

CREATE MATERIALIZED VIEW custom_metrics_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id, name,
       avg(value) AS value,
       max(value) AS value_max
FROM custom_metrics
GROUP BY bucket, host_id, name
WITH NO DATA;

CREATE MATERIALIZED VIEW custom_metrics_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id, name,
       avg(value)     AS value,
       max(value_max) AS value_max
FROM custom_metrics_1m
GROUP BY 1, host_id, name
WITH NO DATA;

-- 1 minute rollups catch up every minute over the last 2 hours,
-- 1 hour rollups every 30 minutes over the last 2 days
SELECT add_continuous_aggregate_policy('host_metrics_1m', start_offset => INTERVAL '2 hours', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('disk_metrics_1m', start_offset => INTERVAL '2 hours', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('disk_io_metrics_1m', start_offset => INTERVAL '2 hours', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('net_metrics_1m', start_offset => INTERVAL '2 hours', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('temp_metrics_1m', start_offset => INTERVAL '2 hours', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('custom_metrics_1m', start_offset => INTERVAL '2 hours', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');

SELECT add_continuous_aggregate_policy('host_metrics_1h', start_offset => INTERVAL '2 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT add_continuous_aggregate_policy('disk_metrics_1h', start_offset => INTERVAL '2 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT add_continuous_aggregate_policy('disk_io_metrics_1h', start_offset => INTERVAL '2 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT add_continuous_aggregate_policy('net_metrics_1h', start_offset => INTERVAL '2 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT add_continuous_aggregate_policy('temp_metrics_1h', start_offset => INTERVAL '2 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT add_continuous_aggregate_policy('custom_metrics_1h', start_offset => INTERVAL '2 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
