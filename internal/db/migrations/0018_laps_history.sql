-- LAPS, Jamf-style: every password a Mac has had (sealed), who viewed each one, an event history,
-- the account's GUID, and an administrator-chosen password waiting to be applied.
ALTER TABLE laps_passwords
    ADD COLUMN guid              TEXT NOT NULL DEFAULT '',
    ADD COLUMN desired_encrypted TEXT,               -- set-password: applied by the agent at its next cycle
    ADD COLUMN desired_by        TEXT NOT NULL DEFAULT '',
    ADD COLUMN desired_at        TIMESTAMPTZ;

CREATE TABLE laps_password_history (
    id                 BIGSERIAL PRIMARY KEY,
    computer_id        UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    account            TEXT NOT NULL,
    guid               TEXT NOT NULL DEFAULT '',
    password_encrypted TEXT NOT NULL,
    fingerprint        TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at         TIMESTAMPTZ                     -- NULL while it is the current password
);
CREATE INDEX laps_password_history_computer_idx ON laps_password_history (computer_id, created_at DESC);

-- Existing current passwords become the first history entry.
INSERT INTO laps_password_history (computer_id, account, password_encrypted, fingerprint, created_at)
SELECT computer_id, account, password_encrypted, fingerprint, COALESCE(escrowed_at, now())
FROM laps_passwords WHERE password_encrypted IS NOT NULL;

CREATE TABLE laps_views (
    id          BIGSERIAL PRIMARY KEY,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL,
    viewed_by   TEXT NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    viewed_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX laps_views_computer_idx ON laps_views (computer_id, viewed_at DESC);

CREATE TABLE laps_events (
    id          BIGSERIAL PRIMARY KEY,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    account     TEXT NOT NULL DEFAULT '',
    event_type  TEXT NOT NULL,                         -- CREATED | ROTATED | VIEWED | PENDING | SET | ERROR
    actor       TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX laps_events_computer_idx ON laps_events (computer_id, at DESC);
