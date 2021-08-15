CREATE TABLE server (
    id BIGSERIAL NOT NULL PRIMARY KEY,
    server_id VARCHAR(100) NOT NULL
);

CREATE TABLE monitor_data (
    id BIGSERIAL NOT NULL PRIMARY KEY,
    server_id BIGINT REFERENCES server(id),
    saved_time VARCHAR(100),
    monitor_type VARCHAR(100),
    monitor_data JSON
);