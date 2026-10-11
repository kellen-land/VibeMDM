-- Secure Token and volume ownership of the LAPS account, as reported by the agent, and one-time
-- grants: an administrator supplies an existing Secure Token admin's credentials, the agent
-- fetches them once (the row is deleted) and runs sysadminctl -secureTokenOn.
ALTER TABLE laps_passwords
    ADD COLUMN secure_token     BOOLEAN,
    ADD COLUMN volume_owner     BOOLEAN,
    ADD COLUMN token_checked_at TIMESTAMPTZ;

CREATE TABLE laps_token_grants (
    computer_id              UUID PRIMARY KEY REFERENCES computers (id) ON DELETE CASCADE,
    admin_user               TEXT NOT NULL,
    admin_password_encrypted TEXT NOT NULL,
    created_by               TEXT NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at               TIMESTAMPTZ NOT NULL
);
