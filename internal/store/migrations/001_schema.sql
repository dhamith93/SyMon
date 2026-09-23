-- Hosts, their latest snapshot, metric hypertables and alerts.
-- Metric tables have no foreign key to hosts, so removing a host keeps its
-- data until retention drops it.

CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE hosts (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text NOT NULL UNIQUE,
    timezone   text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen  timestamptz
);

-- the whole latest snapshot, for system info, disks, interfaces and processes
CREATE TABLE host_latest (
    host_id  bigint PRIMARY KEY REFERENCES hosts (id) ON DELETE CASCADE,
    time     timestamptz NOT NULL,
    snapshot jsonb NOT NULL
);

CREATE TABLE host_metrics (
    time              timestamptz NOT NULL,
    host_id           bigint NOT NULL,
    cpu_pct           double precision,
    load1             double precision,
    load5             double precision,
    load15            double precision,
    mem_used_pct      double precision,
    mem_used_mib      double precision,
    mem_available_mib double precision,
    mem_total_mib     double precision,
    swap_used_pct     double precision,
    swap_used_mib     double precision,
    swap_total_mib    double precision,
    uptime_seconds    double precision,
    psi_cpu_some      double precision,
    psi_memory_some   double precision,
    psi_memory_full   double precision,
    psi_io_some       double precision,
    psi_io_full       double precision,
    tcp_established   double precision,
    tcp_time_wait     double precision,
    tcp_close_wait    double precision,
    tcp_listen        double precision,
    tcp_total         double precision
);
SELECT create_hypertable('host_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON host_metrics (host_id, time DESC);

CREATE TABLE cpu_core_metrics (
    time    timestamptz NOT NULL,
    host_id bigint NOT NULL,
    core    integer NOT NULL,
    pct     double precision
);
SELECT create_hypertable('cpu_core_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON cpu_core_metrics (host_id, time DESC);

CREATE TABLE disk_metrics (
    time            timestamptz NOT NULL,
    host_id         bigint NOT NULL,
    device          text NOT NULL,
    mount           text NOT NULL,
    fstype          text NOT NULL,
    size_bytes      double precision,
    used_bytes      double precision,
    used_pct        double precision,
    inodes_used_pct double precision
);
SELECT create_hypertable('disk_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON disk_metrics (host_id, device, time DESC);

CREATE TABLE disk_io_metrics (
    time       timestamptz NOT NULL,
    host_id    bigint NOT NULL,
    device     text NOT NULL,
    read_bps   double precision,
    write_bps  double precision,
    reads_ps   double precision,
    writes_ps  double precision,
    util_pct   double precision
);
SELECT create_hypertable('disk_io_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON disk_io_metrics (host_id, device, time DESC);

-- rates are null on the agent's first sample of an interface
CREATE TABLE net_metrics (
    time     timestamptz NOT NULL,
    host_id  bigint NOT NULL,
    iface    text NOT NULL,
    state    text NOT NULL DEFAULT '',
    rx_bps   double precision,
    tx_bps   double precision,
    rx_pps   double precision,
    tx_pps   double precision,
    rx_bytes double precision,
    tx_bytes double precision
);
SELECT create_hypertable('net_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON net_metrics (host_id, iface, time DESC);

CREATE TABLE temp_metrics (
    time    timestamptz NOT NULL,
    host_id bigint NOT NULL,
    sensor  text NOT NULL,
    celsius double precision
);
SELECT create_hypertable('temp_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON temp_metrics (host_id, sensor, time DESC);

CREATE TABLE custom_metrics (
    time    timestamptz NOT NULL,
    host_id bigint NOT NULL,
    name    text NOT NULL,
    unit    text NOT NULL DEFAULT '',
    value   double precision
);
SELECT create_hypertable('custom_metrics', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON custom_metrics (host_id, name, time DESC);

CREATE TABLE service_status (
    time    timestamptz NOT NULL,
    host_id bigint NOT NULL,
    name    text NOT NULL,
    running boolean NOT NULL
);
SELECT create_hypertable('service_status', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON service_status (host_id, name, time DESC);

-- top processes at each snapshot, only read for point in time lookups
CREATE TABLE process_snapshots (
    time      timestamptz NOT NULL,
    host_id   bigint NOT NULL,
    processes jsonb NOT NULL
);
SELECT create_hypertable('process_snapshots', by_range('time', INTERVAL '1 day'));
CREATE INDEX ON process_snapshots (host_id, time DESC);

-- one row per incident. target is the disk, service or custom metric name.
CREATE TABLE alerts (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    host_id     bigint NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    rule        text NOT NULL,
    metric      text NOT NULL,
    target      text NOT NULL DEFAULT '',
    severity    smallint NOT NULL,
    value       double precision,
    started_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    resolved_at timestamptz
);
-- at most one open alert per host, rule and target
CREATE UNIQUE INDEX alerts_one_open ON alerts (host_id, rule, metric, target) WHERE resolved_at IS NULL;
CREATE INDEX ON alerts (host_id, started_at DESC);

-- compression, segmented by host so one host's data stays together
ALTER TABLE host_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id');
ALTER TABLE cpu_core_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, core');
ALTER TABLE disk_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, device');
ALTER TABLE disk_io_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, device');
ALTER TABLE net_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, iface');
ALTER TABLE temp_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, sensor');
ALTER TABLE custom_metrics SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, name');
ALTER TABLE service_status SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id, name');
ALTER TABLE process_snapshots SET (timescaledb.compress, timescaledb.compress_segmentby = 'host_id');

SELECT add_compression_policy('host_metrics', INTERVAL '2 days');
SELECT add_compression_policy('cpu_core_metrics', INTERVAL '2 days');
SELECT add_compression_policy('disk_metrics', INTERVAL '2 days');
SELECT add_compression_policy('disk_io_metrics', INTERVAL '2 days');
SELECT add_compression_policy('net_metrics', INTERVAL '2 days');
SELECT add_compression_policy('temp_metrics', INTERVAL '2 days');
SELECT add_compression_policy('custom_metrics', INTERVAL '2 days');
SELECT add_compression_policy('service_status', INTERVAL '2 days');
SELECT add_compression_policy('process_snapshots', INTERVAL '2 days');
