-- Devices read over SNMP by one host's agent each: switches, UPSs, NAS
-- boxes. state is the newest snmp.DeviceReport that could read the device,
-- error why the last report could not (empty when it could). host_id is the
-- host whose agent polls the device, another host cannot report it until
-- the device is removed or that host is.

CREATE TABLE devices (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name             text NOT NULL UNIQUE,
    kind             text NOT NULL DEFAULT '',
    address          text NOT NULL DEFAULT '',
    host_id          bigint REFERENCES hosts (id) ON DELETE SET NULL,
    state            jsonb NOT NULL,
    error            text NOT NULL DEFAULT '',
    interval_seconds integer NOT NULL DEFAULT 60,
    reported_at      timestamptz NOT NULL
);

-- every value a report carried. metric is a profile's name like if_rx_bps
-- or one the user gave an OID, label tells ports and volumes apart.
CREATE TABLE device_metrics (
    time      timestamptz NOT NULL,
    device_id bigint NOT NULL,
    metric    text NOT NULL,
    label     text NOT NULL DEFAULT '',
    unit      text NOT NULL DEFAULT '',
    value     double precision NOT NULL
);
SELECT create_hypertable('device_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON device_metrics (device_id, metric, label, time DESC);

ALTER TABLE device_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'device_id, metric');
SELECT add_compression_policy('device_metrics', INTERVAL '2 days');

-- rollups like custom_metrics: averages, plus the peak for spiky traffic
CREATE MATERIALIZED VIEW device_metrics_1m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 minute', time) AS bucket,
       device_id, metric, label,
       avg(value) AS value,
       max(value) AS value_max
FROM device_metrics
GROUP BY bucket, device_id, metric, label
WITH NO DATA;

CREATE MATERIALIZED VIEW device_metrics_1h
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT time_bucket(INTERVAL '1 hour', bucket) AS bucket,
       device_id, metric, label,
       avg(value)     AS value,
       max(value_max) AS value_max
FROM device_metrics_1m
GROUP BY 1, device_id, metric, label
WITH NO DATA;

-- the wide windows from 003, so data that arrives late still rolls up
SELECT add_continuous_aggregate_policy('device_metrics_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT add_continuous_aggregate_policy('device_metrics_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
