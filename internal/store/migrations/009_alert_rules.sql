-- Alert rules, edited on the dashboard. rule holds the same fields as an
-- alerts.json entry, so there is one rule format everywhere.
CREATE TABLE alert_rules (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text NOT NULL UNIQUE,
    rule       jsonb NOT NULL,
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by text NOT NULL DEFAULT ''
);

-- One row once the rules were set up: alerts.json imported and the default
-- rules added. It only happens once, so a deleted default stays deleted.
CREATE TABLE rule_setup (
    done_at  timestamptz NOT NULL DEFAULT now(),
    -- the imported file, empty when there was none
    source   text NOT NULL,
    imported integer NOT NULL
);
