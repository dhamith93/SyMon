-- When an HTTPS endpoint's certificate expires, null when the check got no
-- certificate
ALTER TABLE endpoint_checks ADD COLUMN cert_expires_at timestamptz;
