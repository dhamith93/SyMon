-- Notifications. Admins set up channels on the dashboard, and the collector
-- queues one row per channel when an alert opens, changes or resolves. The
-- queue is here so a restart loses nothing and retries can wait.
CREATE TABLE notification_channels (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- rules pick channels by name, so it does not change
    name          text NOT NULL UNIQUE,
    -- email, slack, pagerduty or webhook
    kind          text NOT NULL,
    -- includes the secrets, which are never sent to the dashboard
    settings      jsonb NOT NULL,
    -- gets alerts from rules that pick no channel
    is_default    boolean NOT NULL DEFAULT false,
    skip_warnings boolean NOT NULL DEFAULT false,
    enabled       boolean NOT NULL DEFAULT true,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    updated_by    text NOT NULL DEFAULT ''
);

CREATE TABLE notifications (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    alert_id        bigint NOT NULL REFERENCES alerts (id) ON DELETE CASCADE,
    channel_id      bigint NOT NULL REFERENCES notification_channels (id) ON DELETE CASCADE,
    -- opened, changed or resolved
    event           text NOT NULL,
    -- 0 resolved, 1 warning, 2 critical
    severity        smallint NOT NULL,
    value           double precision NOT NULL DEFAULT 0,
    subject         text NOT NULL,
    body            text NOT NULL,
    -- pending, sent or failed
    status          text NOT NULL DEFAULT 'pending',
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text NOT NULL DEFAULT '',
    -- what the channel needs to update its first message, like Slack's ts
    ref             text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz
);
CREATE INDEX ON notifications (next_attempt_at) WHERE status = 'pending';
CREATE INDEX ON notifications (alert_id, channel_id, id);
CREATE INDEX ON notifications (channel_id, id DESC);
