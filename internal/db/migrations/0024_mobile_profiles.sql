-- Mobile Device Configuration Profiles: separate from computer profiles, as in Jamf Pro.
CREATE TABLE mobile_config_profiles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',
    identifier  TEXT NOT NULL UNIQUE,
    uuid        TEXT NOT NULL DEFAULT '',
    payload     TEXT NOT NULL,
    -- Payload editor values ([{type, values}]); NULL for an uploaded .mobileconfig.
    payloads    JSONB,
    scope       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mobile_profile_deployments (
    profile_id       UUID NOT NULL REFERENCES mobile_config_profiles (id) ON DELETE CASCADE,
    mobile_device_id UUID NOT NULL REFERENCES mobile_devices (id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'installed', 'failed', 'removing', 'removed')),
    command_uuid     TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_id, mobile_device_id)
);
CREATE INDEX mobile_profile_deployments_device_idx ON mobile_profile_deployments (mobile_device_id);
CREATE INDEX mobile_profile_deployments_command_idx ON mobile_profile_deployments (command_uuid);
