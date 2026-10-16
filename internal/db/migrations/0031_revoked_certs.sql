-- Identity certificates of deleted devices. /mdm refuses them, so a removed Mac or iPhone
-- cannot quietly re-enroll with the certificate it still holds.
CREATE TABLE IF NOT EXISTS revoked_cert_hashes (
    sha256     TEXT PRIMARY KEY,
    udid       TEXT NOT NULL DEFAULT '',
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
