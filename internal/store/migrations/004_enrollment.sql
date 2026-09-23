-- Enrollment tokens let a new host register itself once, and each host
-- then gets its own credential. Only SHA-256 hashes are stored.

CREATE TABLE enrollment_tokens (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash  bytea NOT NULL UNIQUE,
    -- when set, the token can only enroll a host with this name
    host_name   text,
    uses_left   integer NOT NULL,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE agent_credentials (
    host_id     bigint PRIMARY KEY REFERENCES hosts (id) ON DELETE CASCADE,
    secret_hash bytea NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now()
);
