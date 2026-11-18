-- Finished reads and writes per disk from /proc/diskstats, and the time
-- they took, queueing included. A row covers one agent interval, seconds
-- long. queue_ms is the time IOs spent in flight added up, so queue_ms over
-- the interval is the average queue depth.

CREATE TABLE disk_latency (
    time     timestamptz NOT NULL,
    host_id  bigint NOT NULL,
    device   text NOT NULL,
    seconds  real NOT NULL,
    reads    bigint NOT NULL,
    read_ms  bigint NOT NULL,
    writes   bigint NOT NULL,
    write_ms bigint NOT NULL,
    queue_ms bigint NOT NULL
);
SELECT create_hypertable('disk_latency', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON disk_latency (host_id, device, time DESC);

ALTER TABLE disk_latency SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, device');
SELECT add_compression_policy('disk_latency', INTERVAL '2 days');

-- everything is added up, so an average is the sum of the times over the
-- sum of the IOs at every resolution
CREATE MATERIALIZED VIEW disk_latency_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id,
       device,
       sum(seconds)  AS seconds,
       sum(reads)    AS reads,
       sum(read_ms)  AS read_ms,
       sum(writes)   AS writes,
       sum(write_ms) AS write_ms,
       sum(queue_ms) AS queue_ms
FROM disk_latency
GROUP BY bucket, host_id, device
WITH NO DATA;

CREATE MATERIALIZED VIEW disk_latency_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id,
       device,
       sum(seconds)  AS seconds,
       sum(reads)    AS reads,
       sum(read_ms)  AS read_ms,
       sum(writes)   AS writes,
       sum(write_ms) AS write_ms,
       sum(queue_ms) AS queue_ms
FROM disk_latency_1m
GROUP BY 1, host_id, device
WITH NO DATA;

-- the wide windows from 003, so data that arrives late still rolls up
SELECT add_continuous_aggregate_policy('disk_latency_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('disk_latency_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
