-- Security hardening.

-- Bootstrap / environment-supplied passwords must be changed at first login.
ALTER TABLE admin_users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

-- Purge secrets earlier versions could have copied into the searchable log table.
DELETE FROM server_logs WHERE attrs ? 'password';
DELETE FROM server_logs WHERE msg = 'http' AND attrs->>'path' LIKE '/enroll/configurator/%';
DELETE FROM server_logs WHERE attrs::text ~ 'hooks\.slack\.com|webhook\.office\.com';

-- Single-use, short-lived SCEP challenges minted per enrollment profile download.
CREATE TABLE scep_challenges (
    challenge_hash TEXT PRIMARY KEY,
    purpose        TEXT NOT NULL DEFAULT '',
    single_use     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL,
    used_at        TIMESTAMPTZ
);
CREATE INDEX scep_challenges_expiry_idx ON scep_challenges (expires_at);

-- At most one live remote shell per computer (closes a check-then-act race).
UPDATE shell_sessions SET status = 'closed', reason = 'server upgrade', ended_at = now() WHERE status IN ('pending','active');
CREATE UNIQUE INDEX shell_sessions_one_live_idx ON shell_sessions (computer_id) WHERE status IN ('pending','active');
