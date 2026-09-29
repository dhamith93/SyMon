-- What each agent last said about itself on a ping, and an update an admin
-- asked for. Only agents that can update themselves report their arch. The
-- agent picks an update up on its next ping, and it clears once the agent
-- runs the version asked for.
ALTER TABLE hosts
    ADD COLUMN agent_version       text NOT NULL DEFAULT '',
    ADD COLUMN agent_arch          text NOT NULL DEFAULT '',
    ADD COLUMN update_version      text,
    ADD COLUMN update_url          text,
    ADD COLUMN update_requested_at timestamptz,
    ADD COLUMN update_error        text NOT NULL DEFAULT '';
