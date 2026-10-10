-- FileVault personal recovery key escrow and the MDM command inspector.

CREATE TABLE escrow_identity (
    id         INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    cert_pem   TEXT NOT NULL,
    key_pem    TEXT NOT NULL,      -- may be encrypted at rest (enc:v1:…)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE filevault_keys (
    computer_id   UUID PRIMARY KEY REFERENCES computers (id) ON DELETE CASCADE,
    key_encrypted TEXT NOT NULL,   -- PRK, encrypted at rest when a key is configured
    fingerprint   TEXT NOT NULL DEFAULT '',  -- last 4 characters for display
    source        TEXT NOT NULL DEFAULT 'mdm',
    escrowed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    rotated_count INTEGER NOT NULL DEFAULT 0
);

-- Every step of an MDM command's life, for the inspector timeline.
CREATE TABLE mdm_command_events (
    id            BIGSERIAL PRIMARY KEY,
    command_uuid  TEXT NOT NULL DEFAULT '',
    enrollment_id TEXT NOT NULL,
    event         TEXT NOT NULL,   -- queued, push_sent, push_failed, connected, delivered, not_now, acknowledged, error, cancelled
    details       JSONB NOT NULL DEFAULT '{}'::jsonb,
    at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX mdm_command_events_cmd_idx ON mdm_command_events (command_uuid, at);
CREATE INDEX mdm_command_events_enr_idx ON mdm_command_events (enrollment_id, at DESC);
