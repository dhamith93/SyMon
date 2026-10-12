-- Host tasks. hosts.tasks is the list from the host's own tasks file, as
-- its agent last sent it. A run is asked for by an admin on the dashboard,
-- picked up by the agent and reported back.
ALTER TABLE hosts ADD COLUMN tasks jsonb NOT NULL DEFAULT '[]';

CREATE TABLE task_runs (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    host_id         bigint NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    task            text NOT NULL,
    -- the command the admin saw, the agent runs it only when its file agrees
    command         text NOT NULL,
    timeout_seconds integer NOT NULL,
    -- queued, running, succeeded, failed, timed_out, expired or lost
    status          text NOT NULL DEFAULT 'queued',
    requested_by    text NOT NULL,
    requested_at    timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz,
    finished_at     timestamptz,
    exit_code       integer,
    error           text NOT NULL DEFAULT '',
    output          text NOT NULL DEFAULT '',
    truncated       boolean NOT NULL DEFAULT false
);
-- a host runs one task at a time
CREATE UNIQUE INDEX task_runs_one_active ON task_runs (host_id) WHERE status IN ('queued', 'running');
CREATE INDEX ON task_runs (host_id, id DESC);
