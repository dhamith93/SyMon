-- HTTP checks the collector runs for endpoint alert rules. name is the
-- rule's name. status_code and latency_ms are null when there was no
-- response. Kept as long as the hourly rollups, since a check every few
-- minutes is little data.

CREATE TABLE endpoint_checks (
    time        timestamptz NOT NULL,
    name        text NOT NULL,
    url         text NOT NULL,
    method      text NOT NULL,
    status_code integer,
    ok          boolean NOT NULL,
    latency_ms  double precision,
    error       text NOT NULL DEFAULT ''
);
SELECT create_hypertable('endpoint_checks', by_range('time', INTERVAL '7 days'));
CREATE INDEX ON endpoint_checks (name, time DESC);

ALTER TABLE endpoint_checks SET (timescaledb.compress, timescaledb.compress_segmentby = 'name');
SELECT add_compression_policy('endpoint_checks', INTERVAL '2 days');

-- endpoint alerts belong to no host. NULLS NOT DISTINCT keeps at most one
-- open alert per endpoint rule too.
ALTER TABLE alerts ALTER COLUMN host_id DROP NOT NULL;
DROP INDEX alerts_one_open;
CREATE UNIQUE INDEX alerts_one_open ON alerts (host_id, rule, metric, target) NULLS NOT DISTINCT WHERE resolved_at IS NULL;
