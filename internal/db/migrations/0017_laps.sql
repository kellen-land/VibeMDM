-- Local administrator password solution (LAPS): fleet config and per-Mac escrowed passwords.
CREATE TABLE laps_config (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    config     JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE laps_passwords (
    computer_id         UUID PRIMARY KEY REFERENCES computers (id) ON DELETE CASCADE,
    account             TEXT NOT NULL DEFAULT '',
    password_encrypted  TEXT,                          -- current, confirmed-applied password (sealed)
    fingerprint         TEXT NOT NULL DEFAULT '',
    escrowed_at         TIMESTAMPTZ,
    rotated_count       INTEGER NOT NULL DEFAULT 0,
    -- A newly generated password is staged here BEFORE the agent changes the account, so a
    -- failed upload can never leave the Mac with a password the server does not know.
    pending_encrypted   TEXT,
    pending_fingerprint TEXT NOT NULL DEFAULT '',
    pending_account     TEXT NOT NULL DEFAULT '',
    pending_at          TIMESTAMPTZ,
    last_viewed_at      TIMESTAMPTZ,
    last_viewed_by      TEXT NOT NULL DEFAULT '',
    rotate_after        TIMESTAMPTZ,                   -- set by a reveal: rotate once this passes
    rotate_requested    BOOLEAN NOT NULL DEFAULT FALSE,
    last_error          TEXT NOT NULL DEFAULT '',
    last_error_at       TIMESTAMPTZ
);
