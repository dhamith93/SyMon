-- Dashboard users. Passwords are PBKDF2-SHA256 hashes with a salt of their
-- own. iterations is kept per user so it can be raised later.

CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          text NOT NULL UNIQUE,
    password_hash bytea NOT NULL,
    salt          bytea NOT NULL,
    iterations    integer NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz
);

-- Logged in browsers. The token is random, so only its SHA-256 is kept.
CREATE TABLE user_sessions (
    token_hash bytea PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX ON user_sessions (user_id);
