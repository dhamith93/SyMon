-- Network traffic per program from the agent's eBPF counters, loopback left
-- out. A snapshot has a row for each of the busiest programs over one agent
-- interval, seconds long, and one with an empty name for the rest, so its
-- rows add up to the host's traffic.

CREATE TABLE process_network (
    time     timestamptz NOT NULL,
    host_id  bigint NOT NULL,
    name     text NOT NULL,
    seconds  real NOT NULL,
    rx_bytes bigint NOT NULL,
    tx_bytes bigint NOT NULL
);
SELECT create_hypertable('process_network', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON process_network (host_id, time DESC);

ALTER TABLE process_network SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id');
SELECT add_compression_policy('process_network', INTERVAL '2 days');

-- bytes and seconds are added up, so a rate is bytes over the bucket at
-- every resolution. The peaks are the fastest single interval.
CREATE MATERIALIZED VIEW process_network_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id,
       name,
       sum(seconds)  AS seconds,
       sum(rx_bytes) AS rx_bytes,
       sum(tx_bytes) AS tx_bytes,
       max(rx_bytes::float8 / nullif(seconds, 0)) AS rx_bps_max,
       max(tx_bytes::float8 / nullif(seconds, 0)) AS tx_bps_max
FROM process_network
GROUP BY bucket, host_id, name
WITH NO DATA;

CREATE MATERIALIZED VIEW process_network_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id,
       name,
       sum(seconds)    AS seconds,
       sum(rx_bytes)   AS rx_bytes,
       sum(tx_bytes)   AS tx_bytes,
       max(rx_bps_max) AS rx_bps_max,
       max(tx_bps_max) AS tx_bps_max
FROM process_network_1m
GROUP BY 1, host_id, name
WITH NO DATA;

-- the wide windows from 003, so data that arrives late still rolls up
SELECT add_continuous_aggregate_policy('process_network_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('process_network_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
