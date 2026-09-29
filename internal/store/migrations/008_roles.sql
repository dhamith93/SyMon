-- Admins can change alert rules, viewers can only look. Users from before
-- roles become admins, since they could do everything before.
ALTER TABLE users ADD COLUMN role text NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'viewer'));
