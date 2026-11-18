-- Process activity from the agent's eBPF tracer. Each snapshot carries the
-- counts for one agent interval, seconds long, and the processes that ended
-- badly in it: a non-zero status, a signal or the OOM killer.

CREATE TABLE process_activity (
    time          timestamptz NOT NULL,
    host_id       bigint NOT NULL,
    seconds       real NOT NULL,
    execs         integer NOT NULL,
    failed_execs  integer NOT NULL,
    exits         integer NOT NULL,
    nonzero_exits integer NOT NULL,
    signaled      integer NOT NULL,
    dropped       integer NOT NULL
);
SELECT create_hypertable('process_activity', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON process_activity (host_id, time DESC);

ALTER TABLE process_activity SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id');
SELECT add_compression_policy('process_activity', INTERVAL '2 days');

-- counts are added up, seconds too, so a rate is sum(count) / sum(seconds)
-- at every resolution, even when the agent missed some intervals
CREATE MATERIALIZED VIEW process_activity_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       host_id,
       sum(seconds)       AS seconds,
       sum(execs)         AS execs,
       sum(failed_execs)  AS failed_execs,
       sum(exits)         AS exits,
       sum(nonzero_exits) AS nonzero_exits,
       sum(signaled)      AS signaled,
       sum(dropped)       AS dropped
FROM process_activity
GROUP BY bucket, host_id
WITH NO DATA;

CREATE MATERIALIZED VIEW process_activity_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       host_id,
       sum(seconds)       AS seconds,
       sum(execs)         AS execs,
       sum(failed_execs)  AS failed_execs,
       sum(exits)         AS exits,
       sum(nonzero_exits) AS nonzero_exits,
       sum(signaled)      AS signaled,
       sum(dropped)       AS dropped
FROM process_activity_1m
GROUP BY 1, host_id
WITH NO DATA;

-- the wide windows from 003, so data that arrives late still rolls up
SELECT add_continuous_aggregate_policy('process_activity_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('process_activity_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');

-- every process that ended badly, at the time the agent saw it end. status
-- is 0 when a signal ended it, signal is 0 when it exited by itself.
CREATE TABLE process_exits (
    time             timestamptz NOT NULL,
    host_id          bigint NOT NULL,
    name             text NOT NULL,
    parent           text NOT NULL DEFAULT '',
    pid              integer NOT NULL,
    ppid             integer NOT NULL,
    uid              bigint NOT NULL,
    status           smallint NOT NULL,
    signal           smallint NOT NULL,
    core_dumped      boolean NOT NULL DEFAULT false,
    oom              boolean NOT NULL DEFAULT false,
    lifetime_seconds real NOT NULL
);
SELECT create_hypertable('process_exits', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON process_exits (host_id, time DESC);

ALTER TABLE process_exits SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id');
SELECT add_compression_policy('process_exits', INTERVAL '2 days');
