-- VibeMDM core schema. Lives alongside the NanoMDM tables from 0001.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL DEFAULT '',
    email         TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'viewer')),
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES admin_users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'viewer')),
    created_by   UUID REFERENCES admin_users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ
);

-- SCEP certificate authority used to issue MDM identity certificates.
CREATE TABLE scep_ca (
    id         INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    cert_pem   TEXT NOT NULL,
    key_pem    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE SEQUENCE scep_serial_seq START WITH 1000;

CREATE TABLE scep_certs (
    serial      NUMERIC PRIMARY KEY,
    common_name TEXT NOT NULL,
    cert_pem    TEXT NOT NULL,
    not_before  TIMESTAMPTZ NOT NULL,
    not_after   TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX scep_certs_cn_idx ON scep_certs (common_name);

-- Key material for the APNs push certificate request (mdmcert.download flow).
CREATE TABLE push_cert_requests (
    id         INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    key_pem    TEXT NOT NULL,
    csr_pem    TEXT NOT NULL,
    cert_pem   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE enrollment_invitations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL DEFAULT '',
    token_hash   TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    max_uses     INTEGER,
    uses         INTEGER NOT NULL DEFAULT 0,
    expires_at   TIMESTAMPTZ,
    created_by   UUID REFERENCES admin_users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A computer is the unified record for a managed Mac: it joins the MDM
-- enrollment (NanoMDM enrollment ID / UDID) and the agent enrollment.
CREATE TABLE computers (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    udid                    TEXT UNIQUE,
    serial_number           TEXT,
    name                    TEXT NOT NULL DEFAULT '',
    hostname                TEXT NOT NULL DEFAULT '',
    local_hostname          TEXT NOT NULL DEFAULT '',
    model                   TEXT NOT NULL DEFAULT '',
    model_name              TEXT NOT NULL DEFAULT '',
    model_identifier        TEXT NOT NULL DEFAULT '',
    product_name            TEXT NOT NULL DEFAULT '',
    os_version              TEXT NOT NULL DEFAULT '',
    os_build                TEXT NOT NULL DEFAULT '',
    architecture            TEXT NOT NULL DEFAULT '',
    cpu                     TEXT NOT NULL DEFAULT '',
    cpu_cores               INTEGER,
    memory_mb               BIGINT,
    disk_total_mb           BIGINT,
    disk_free_mb            BIGINT,
    ip_address              TEXT NOT NULL DEFAULT '',
    mac_address             TEXT NOT NULL DEFAULT '',
    last_user               TEXT NOT NULL DEFAULT '',
    last_user_full_name     TEXT NOT NULL DEFAULT '',
    filevault_enabled       BOOLEAN,
    sip_enabled             BOOLEAN,
    firewall_enabled        BOOLEAN,
    gatekeeper_enabled      BOOLEAN,
    secure_boot_level       TEXT NOT NULL DEFAULT '',
    is_supervised           BOOLEAN,
    activation_lock_enabled BOOLEAN,
    bootstrap_token_escrowed BOOLEAN,
    battery_percent         INTEGER,
    uptime_seconds          BIGINT,
    mdm_enrolled            BOOLEAN NOT NULL DEFAULT FALSE,
    mdm_enrolled_at         TIMESTAMPTZ,
    mdm_last_checkin        TIMESTAMPTZ,
    mdm_unenrolled_at       TIMESTAMPTZ,
    mdm_profile_identifier  TEXT NOT NULL DEFAULT '',
    agent_enrolled          BOOLEAN NOT NULL DEFAULT FALSE,
    agent_enrolled_at       TIMESTAMPTZ,
    agent_last_checkin      TIMESTAMPTZ,
    agent_version           TEXT NOT NULL DEFAULT '',
    agent_token_hash        TEXT UNIQUE,
    enrollment_complete_fired BOOLEAN NOT NULL DEFAULT FALSE,
    mdm_device_info         JSONB NOT NULL DEFAULT '{}'::jsonb,
    security_info           JSONB NOT NULL DEFAULT '{}'::jsonb,
    hardware                JSONB NOT NULL DEFAULT '{}'::jsonb,
    inventory               JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_inventory_at       TIMESTAMPTZ,
    asset_tag               TEXT NOT NULL DEFAULT '',
    site                    TEXT NOT NULL DEFAULT '',
    department              TEXT NOT NULL DEFAULT '',
    building                TEXT NOT NULL DEFAULT '',
    room                    TEXT NOT NULL DEFAULT '',
    assigned_user           TEXT NOT NULL DEFAULT '',
    assigned_real_name      TEXT NOT NULL DEFAULT '',
    assigned_email          TEXT NOT NULL DEFAULT '',
    assigned_phone          TEXT NOT NULL DEFAULT '',
    assigned_position       TEXT NOT NULL DEFAULT '',
    notes                   TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX computers_serial_idx ON computers (serial_number);
CREATE INDEX computers_name_idx ON computers (lower(name));

CREATE TABLE computer_apps (
    id          BIGSERIAL PRIMARY KEY,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    version     TEXT NOT NULL DEFAULT '',
    bundle_id   TEXT NOT NULL DEFAULT '',
    path        TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT 'agent'
);
CREATE INDEX computer_apps_computer_idx ON computer_apps (computer_id);
CREATE INDEX computer_apps_name_idx ON computer_apps (lower(name));

CREATE TABLE computer_profiles (
    id           BIGSERIAL PRIMARY KEY,
    computer_id  UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    identifier   TEXT NOT NULL,
    uuid         TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    organization TEXT NOT NULL DEFAULT '',
    is_managed   BOOLEAN NOT NULL DEFAULT FALSE,
    source       TEXT NOT NULL DEFAULT 'mdm'
);
CREATE INDEX computer_profiles_computer_idx ON computer_profiles (computer_id);

CREATE TABLE computer_local_users (
    id          BIGSERIAL PRIMARY KEY,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    username    TEXT NOT NULL,
    uid         INTEGER,
    real_name   TEXT NOT NULL DEFAULT '',
    home        TEXT NOT NULL DEFAULT '',
    is_admin    BOOLEAN NOT NULL DEFAULT FALSE,
    secure_token BOOLEAN
);
CREATE INDEX computer_local_users_computer_idx ON computer_local_users (computer_id);

CREATE TABLE extension_attributes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    data_type   TEXT NOT NULL DEFAULT 'string' CHECK (data_type IN ('string', 'integer', 'date')),
    input_type  TEXT NOT NULL DEFAULT 'script' CHECK (input_type IN ('script', 'text')),
    script      TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE computer_ea_values (
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    ea_id       UUID NOT NULL REFERENCES extension_attributes (id) ON DELETE CASCADE,
    value       TEXT NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (computer_id, ea_id)
);

CREATE TABLE computer_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_smart    BOOLEAN NOT NULL DEFAULT FALSE,
    criteria    JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE computer_group_members (
    group_id    UUID NOT NULL REFERENCES computer_groups (id) ON DELETE CASCADE,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, computer_id)
);

CREATE TABLE scripts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL UNIQUE,
    category         TEXT NOT NULL DEFAULT '',
    info             TEXT NOT NULL DEFAULT '',
    notes            TEXT NOT NULL DEFAULT '',
    priority         TEXT NOT NULL DEFAULT 'after' CHECK (priority IN ('before', 'after')),
    parameter_labels JSONB NOT NULL DEFAULT '["","","","","","","",""]'::jsonb,
    contents         TEXT NOT NULL DEFAULT '',
    os_requirements  TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE packages (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    filename     TEXT NOT NULL UNIQUE,
    category     TEXT NOT NULL DEFAULT '',
    info         TEXT NOT NULL DEFAULT '',
    notes        TEXT NOT NULL DEFAULT '',
    size_bytes   BIGINT NOT NULL DEFAULT 0,
    sha256       TEXT NOT NULL DEFAULT '',
    priority     INTEGER NOT NULL DEFAULT 10,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE config_profiles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    description  TEXT NOT NULL DEFAULT '',
    category     TEXT NOT NULL DEFAULT '',
    identifier   TEXT NOT NULL UNIQUE,
    uuid         TEXT NOT NULL DEFAULT '',
    level        TEXT NOT NULL DEFAULT 'device' CHECK (level IN ('device', 'user')),
    payload      TEXT NOT NULL,
    scope        JSONB NOT NULL DEFAULT '{"all_computers":false,"computer_ids":[],"group_ids":[],"excluded_computer_ids":[],"excluded_group_ids":[]}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profile_deployments (
    profile_id   UUID NOT NULL REFERENCES config_profiles (id) ON DELETE CASCADE,
    computer_id  UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'installed', 'failed', 'removing', 'removed')),
    command_uuid TEXT NOT NULL DEFAULT '',
    error        TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, computer_id)
);
CREATE INDEX profile_deployments_computer_idx ON profile_deployments (computer_id);

CREATE TABLE policies (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL UNIQUE,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    category         TEXT NOT NULL DEFAULT '',
    triggers         JSONB NOT NULL DEFAULT '{"startup":false,"login":false,"checkin":true,"enrollment_complete":false,"network_change":false,"custom":""}'::jsonb,
    frequency        TEXT NOT NULL DEFAULT 'once_per_computer' CHECK (frequency IN ('ongoing', 'once_per_computer', 'once_per_day', 'once_per_week', 'once_per_month')),
    scope            JSONB NOT NULL DEFAULT '{"all_computers":false,"computer_ids":[],"group_ids":[],"excluded_computer_ids":[],"excluded_group_ids":[]}'::jsonb,
    scripts          JSONB NOT NULL DEFAULT '[]'::jsonb,
    packages         JSONB NOT NULL DEFAULT '[]'::jsonb,
    run_command      TEXT NOT NULL DEFAULT '',
    update_inventory BOOLEAN NOT NULL DEFAULT FALSE,
    reboot           JSONB NOT NULL DEFAULT '{"enabled":false,"message":"","delay_minutes":0}'::jsonb,
    user_interaction JSONB NOT NULL DEFAULT '{"start_message":"","complete_message":""}'::jsonb,
    self_service     JSONB NOT NULL DEFAULT '{"enabled":false,"display_name":"","description":"","button_text":"Install"}'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE policy_logs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id    UUID NOT NULL REFERENCES policies (id) ON DELETE CASCADE,
    computer_id  UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    trigger      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    log          TEXT NOT NULL DEFAULT '',
    steps        JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX policy_logs_policy_computer_idx ON policy_logs (policy_id, computer_id, created_at DESC);
CREATE INDEX policy_logs_computer_idx ON policy_logs (computer_id, created_at DESC);

CREATE TABLE agent_tasks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    computer_id  UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    type         TEXT NOT NULL,
    payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    status       TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'completed', 'failed', 'cancelled')),
    result       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at      TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);
CREATE INDEX agent_tasks_computer_status_idx ON agent_tasks (computer_id, status, created_at);

-- Metadata about MDM commands we enqueue, so results can be interpreted.
CREATE TABLE mdm_commands_meta (
    command_uuid  TEXT PRIMARY KEY,
    computer_id   UUID REFERENCES computers (id) ON DELETE CASCADE,
    enrollment_id TEXT NOT NULL,
    request_type  TEXT NOT NULL,
    kind          TEXT NOT NULL DEFAULT '',
    ref_id        UUID,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX mdm_commands_meta_computer_idx ON mdm_commands_meta (computer_id, created_at DESC);

CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor       TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT '',
    target_id   TEXT NOT NULL DEFAULT '',
    details     JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX audit_log_at_idx ON audit_log (at DESC);

-- Helper used by smart group version comparisons: "14.2.1" -> {14,2,1}.
CREATE FUNCTION vibemdm_version_array(v TEXT) RETURNS INT[] IMMUTABLE LANGUAGE SQL AS $$
    SELECT COALESCE(array_agg(COALESCE(NULLIF(regexp_replace(p, '\D', '', 'g'), '')::INT, 0) ORDER BY ord), '{}'::INT[])
    FROM unnest(string_to_array(COALESCE(v, ''), '.')) WITH ORDINALITY AS t(p, ord)
$$;
