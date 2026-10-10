-- Native S.U.P.E.R.M.A.N. (super) deployment: one fleet-wide configuration.
CREATE TABLE super_config (
    id                   INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled              BOOLEAN NOT NULL DEFAULT FALSE,
    scope                JSONB NOT NULL DEFAULT '{}'::jsonb,
    auth_mode            TEXT NOT NULL DEFAULT 'none' CHECK (auth_mode IN ('none','local','service')),
    auth_account         TEXT NOT NULL DEFAULT '',
    auth_password        TEXT NOT NULL DEFAULT '',   -- sealed when an encryption key is configured
    profile_id           UUID REFERENCES config_profiles (id) ON DELETE SET NULL,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
