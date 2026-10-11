-- Automatic agent installation through MDM (InstallEnterpriseApplication).

-- The CA that signs the agent package's signing certificate. Its root is sent to a Mac only
-- inside the temporary bootstrap profile, so macOS trusts the package while it installs.
CREATE TABLE pkg_signing_ca (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    cert_pem   TEXT NOT NULL,
    key_pem    TEXT NOT NULL,          -- sealed with VIBEMDM_ENCRYPTION_KEY
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The uploaded, signed agent package served to mdmclient.
CREATE TABLE agent_package (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    file_name   TEXT NOT NULL,
    version     TEXT NOT NULL,
    size        BIGINT NOT NULL,
    sha256      TEXT NOT NULL,
    md5_chunk   BIGINT NOT NULL,
    md5s        JSONB NOT NULL DEFAULT '[]'::jsonb,
    signed_by   TEXT NOT NULL DEFAULT '',  -- signer common name, when recognisable
    uploaded_by TEXT NOT NULL DEFAULT '',
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Single-use agent enrollment tokens delivered to one Mac in the bootstrap profile.
CREATE TABLE agent_bootstrap_tokens (
    token_hash  TEXT PRIMARY KEY,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ
);
CREATE INDEX agent_bootstrap_tokens_computer_idx ON agent_bootstrap_tokens (computer_id);

-- Per-Mac install attempts, so the console and Health can show what happened.
CREATE TABLE agent_install_attempts (
    id          BIGSERIAL PRIMARY KEY,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    command_uuid TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'sent' CHECK (status IN ('sent','installed','failed')),
    detail      TEXT NOT NULL DEFAULT '',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX agent_install_attempts_computer_idx ON agent_install_attempts (computer_id, created_at DESC);
