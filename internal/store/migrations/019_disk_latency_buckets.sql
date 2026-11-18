-- Disk latency histograms from the agent's eBPF tracer: how many of a whole
-- disk's reads or writes took each bucket's time over one agent interval.
-- b0 is under 16 µs, bk from 16·2^(k-1) µs up to 16·2^k µs, and b18 is
-- 2.1 s and more. A row is only written when the disk did that op.

CREATE TABLE disk_latency_buckets (
    time    timestamptz NOT NULL,
    host_id bigint NOT NULL,
    device  text NOT NULL,
    op      text NOT NULL,
    b0  bigint NOT NULL,
    b1  bigint NOT NULL,
    b2  bigint NOT NULL,
    b3  bigint NOT NULL,
    b4  bigint NOT NULL,
    b5  bigint NOT NULL,
    b6  bigint NOT NULL,
    b7  bigint NOT NULL,
    b8  bigint NOT NULL,
    b9  bigint NOT NULL,
    b10 bigint NOT NULL,
    b11 bigint NOT NULL,
    b12 bigint NOT NULL,
    b13 bigint NOT NULL,
    b14 bigint NOT NULL,
    b15 bigint NOT NULL,
    b16 bigint NOT NULL,
    b17 bigint NOT NULL,
    b18 bigint NOT NULL
);
SELECT create_hypertable('disk_latency_buckets', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON disk_latency_buckets (host_id, device, time DESC);

ALTER TABLE disk_latency_buckets SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, device, op');
SELECT add_compression_policy('disk_latency_buckets', INTERVAL '2 days');

-- buckets are added up, so quantiles can be read at every resolution
CREATE MATERIALIZED VIEW disk_latency_buckets_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id,
       device,
       op,
       sum(b0) AS b0,
       sum(b1) AS b1,
       sum(b2) AS b2,
       sum(b3) AS b3,
       sum(b4) AS b4,
       sum(b5) AS b5,
       sum(b6) AS b6,
       sum(b7) AS b7,
       sum(b8) AS b8,
       sum(b9) AS b9,
       sum(b10) AS b10,
       sum(b11) AS b11,
       sum(b12) AS b12,
       sum(b13) AS b13,
       sum(b14) AS b14,
       sum(b15) AS b15,
       sum(b16) AS b16,
       sum(b17) AS b17,
       sum(b18) AS b18
FROM disk_latency_buckets
GROUP BY bucket, host_id, device, op
WITH NO DATA;

CREATE MATERIALIZED VIEW disk_latency_buckets_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id,
       device,
       op,
       sum(b0) AS b0,
       sum(b1) AS b1,
       sum(b2) AS b2,
       sum(b3) AS b3,
       sum(b4) AS b4,
       sum(b5) AS b5,
       sum(b6) AS b6,
       sum(b7) AS b7,
       sum(b8) AS b8,
       sum(b9) AS b9,
       sum(b10) AS b10,
       sum(b11) AS b11,
       sum(b12) AS b12,
       sum(b13) AS b13,
       sum(b14) AS b14,
       sum(b15) AS b15,
       sum(b16) AS b16,
       sum(b17) AS b17,
       sum(b18) AS b18
FROM disk_latency_buckets_1m
GROUP BY 1, host_id, device, op
WITH NO DATA;

-- the wide windows from 003, so data that arrives late still rolls up
SELECT add_continuous_aggregate_policy('disk_latency_buckets_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('disk_latency_buckets_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
