-- Wider refresh windows. With 2 hours, data that arrived later than that,
-- for example after a collector outage, was never rolled up and did not
-- show on charts that read the rollups. Refreshes only recompute buckets
-- whose raw data changed, so a wider window costs little.

SELECT remove_continuous_aggregate_policy('host_metrics_1m');
SELECT add_continuous_aggregate_policy('host_metrics_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT remove_continuous_aggregate_policy('disk_metrics_1m');
SELECT add_continuous_aggregate_policy('disk_metrics_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT remove_continuous_aggregate_policy('disk_io_metrics_1m');
SELECT add_continuous_aggregate_policy('disk_io_metrics_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT remove_continuous_aggregate_policy('net_metrics_1m');
SELECT add_continuous_aggregate_policy('net_metrics_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT remove_continuous_aggregate_policy('temp_metrics_1m');
SELECT add_continuous_aggregate_policy('temp_metrics_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');
SELECT remove_continuous_aggregate_policy('custom_metrics_1m');
SELECT add_continuous_aggregate_policy('custom_metrics_1m', start_offset => INTERVAL '1 day', end_offset => INTERVAL '1 minute', schedule_interval => INTERVAL '1 minute');

SELECT remove_continuous_aggregate_policy('host_metrics_1h');
SELECT add_continuous_aggregate_policy('host_metrics_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT remove_continuous_aggregate_policy('disk_metrics_1h');
SELECT add_continuous_aggregate_policy('disk_metrics_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT remove_continuous_aggregate_policy('disk_io_metrics_1h');
SELECT add_continuous_aggregate_policy('disk_io_metrics_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT remove_continuous_aggregate_policy('net_metrics_1h');
SELECT add_continuous_aggregate_policy('net_metrics_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT remove_continuous_aggregate_policy('temp_metrics_1h');
SELECT add_continuous_aggregate_policy('temp_metrics_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
SELECT remove_continuous_aggregate_policy('custom_metrics_1h');
SELECT add_continuous_aggregate_policy('custom_metrics_1h', start_offset => INTERVAL '3 days', end_offset => INTERVAL '1 hour', schedule_interval => INTERVAL '30 minutes');
