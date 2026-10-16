-- Previous FileVault recovery keys (sealed like filevault_keys.key_encrypted). A newly reported
-- key never destroys the old one, so a bad or unexpected report cannot lose a working key.
CREATE TABLE IF NOT EXISTS filevault_key_history (
    id            BIGSERIAL PRIMARY KEY,
    computer_id   UUID NOT NULL REFERENCES computers(id) ON DELETE CASCADE,
    key_encrypted TEXT NOT NULL,
    fingerprint   TEXT NOT NULL DEFAULT '',
    source        TEXT NOT NULL DEFAULT '',
    escrowed_at   TIMESTAMPTZ NOT NULL,
    replaced_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS filevault_key_history_computer_idx ON filevault_key_history (computer_id, replaced_at DESC);
