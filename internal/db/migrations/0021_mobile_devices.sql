-- Mobile devices (iPhone, iPad, iPod touch, Apple TV) live in their own tables, separate from
-- computers, the way Jamf Pro keeps Computers and Mobile Devices apart.
CREATE TABLE mobile_devices (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    udid                    TEXT NOT NULL UNIQUE,
    serial_number           TEXT,
    name                    TEXT NOT NULL DEFAULT '',
    device_type             TEXT NOT NULL DEFAULT '',   -- iPhone | iPad | iPod | AppleTV
    model                   TEXT NOT NULL DEFAULT '',   -- e.g. iPhone14,4
    model_name              TEXT NOT NULL DEFAULT '',   -- e.g. iPhone 13 mini
    product_name            TEXT NOT NULL DEFAULT '',
    os_version              TEXT NOT NULL DEFAULT '',
    os_build                TEXT NOT NULL DEFAULT '',
    capacity_gb             NUMERIC,
    available_gb            NUMERIC,
    battery_level           NUMERIC,                    -- 0..1
    wifi_mac                TEXT NOT NULL DEFAULT '',
    bluetooth_mac           TEXT NOT NULL DEFAULT '',
    imei                    TEXT NOT NULL DEFAULT '',
    meid                    TEXT NOT NULL DEFAULT '',
    phone_number            TEXT NOT NULL DEFAULT '',
    carrier                 TEXT NOT NULL DEFAULT '',
    cellular_technology     TEXT NOT NULL DEFAULT '',
    is_roaming              BOOLEAN,
    is_supervised           BOOLEAN,
    is_shared_ipad          BOOLEAN,
    activation_lock_enabled BOOLEAN,
    find_my_enabled         BOOLEAN,
    lost_mode_enabled       BOOLEAN,
    passcode_present        BOOLEAN,
    passcode_compliant      BOOLEAN,
    hardware_encryption     INTEGER,                    -- SecurityInfo HardwareEncryptionCaps
    last_backup_at          TIMESTAMPTZ,
    mdm_enrolled            BOOLEAN NOT NULL DEFAULT FALSE,
    mdm_enrolled_at         TIMESTAMPTZ,
    mdm_last_checkin        TIMESTAMPTZ,
    mdm_unenrolled_at       TIMESTAMPTZ,
    device_info             JSONB NOT NULL DEFAULT '{}'::jsonb,
    security_info           JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_inventory_at       TIMESTAMPTZ,
    asset_tag               TEXT NOT NULL DEFAULT '',
    site                    TEXT NOT NULL DEFAULT '',
    department              TEXT NOT NULL DEFAULT '',
    building                TEXT NOT NULL DEFAULT '',
    room                    TEXT NOT NULL DEFAULT '',
    assigned_user           TEXT NOT NULL DEFAULT '',
    assigned_real_name      TEXT NOT NULL DEFAULT '',
    assigned_email          TEXT NOT NULL DEFAULT '',
    notes                   TEXT NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX mobile_devices_serial_idx ON mobile_devices (serial_number);
CREATE INDEX mobile_devices_name_idx ON mobile_devices (lower(name));

CREATE TABLE mobile_device_apps (
    id               BIGSERIAL PRIMARY KEY,
    mobile_device_id UUID NOT NULL REFERENCES mobile_devices (id) ON DELETE CASCADE,
    name             TEXT NOT NULL DEFAULT '',
    version          TEXT NOT NULL DEFAULT '',
    bundle_id        TEXT NOT NULL DEFAULT '',
    managed          BOOLEAN NOT NULL DEFAULT FALSE,
    managed_status   TEXT NOT NULL DEFAULT ''        -- ManagedApplicationList Status, e.g. Managed, Installing
);
CREATE INDEX mobile_device_apps_device_idx ON mobile_device_apps (mobile_device_id);

CREATE TABLE mobile_device_installed_profiles (
    id               BIGSERIAL PRIMARY KEY,
    mobile_device_id UUID NOT NULL REFERENCES mobile_devices (id) ON DELETE CASCADE,
    identifier       TEXT NOT NULL DEFAULT '',
    uuid             TEXT NOT NULL DEFAULT '',
    display_name     TEXT NOT NULL DEFAULT '',
    organization     TEXT NOT NULL DEFAULT '',
    is_managed       BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX mobile_device_installed_profiles_device_idx ON mobile_device_installed_profiles (mobile_device_id);

CREATE TABLE mobile_device_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_smart    BOOLEAN NOT NULL DEFAULT FALSE,
    criteria    JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mobile_device_group_members (
    group_id         UUID NOT NULL REFERENCES mobile_device_groups (id) ON DELETE CASCADE,
    mobile_device_id UUID NOT NULL REFERENCES mobile_devices (id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, mobile_device_id)
);

-- MDM commands can belong to a mobile device instead of a computer.
ALTER TABLE mdm_commands_meta ADD COLUMN mobile_device_id UUID REFERENCES mobile_devices (id) ON DELETE CASCADE;
CREATE INDEX mdm_commands_meta_mobile_idx ON mdm_commands_meta (mobile_device_id, created_at DESC);

-- Move iPhones and iPads that earlier versions filed under computers.
INSERT INTO mobile_devices (udid, serial_number, name, device_type, model, model_name, product_name, os_version, os_build,
    is_supervised, activation_lock_enabled, mdm_enrolled, mdm_enrolled_at, mdm_last_checkin, mdm_unenrolled_at,
    device_info, security_info, last_inventory_at, asset_tag, site, department, building, room, assigned_user,
    assigned_real_name, assigned_email, notes, created_at, updated_at)
SELECT udid, serial_number, name,
    CASE WHEN product_name LIKE 'iPhone%' THEN 'iPhone' WHEN product_name LIKE 'iPad%' THEN 'iPad'
         WHEN product_name LIKE 'iPod%' THEN 'iPod' ELSE 'AppleTV' END,
    model, model_name, product_name, os_version, os_build, is_supervised, activation_lock_enabled, mdm_enrolled,
    mdm_enrolled_at, mdm_last_checkin, mdm_unenrolled_at, mdm_device_info, security_info, last_inventory_at, asset_tag,
    site, department, building, room, assigned_user, assigned_real_name, assigned_email, notes, created_at, updated_at
FROM computers
WHERE udid IS NOT NULL AND (product_name LIKE 'iPhone%' OR product_name LIKE 'iPad%' OR product_name LIKE 'iPod%' OR product_name LIKE 'AppleTV%');

UPDATE mdm_commands_meta m SET mobile_device_id = md.id, computer_id = NULL
FROM computers c JOIN mobile_devices md ON md.udid = c.udid
WHERE m.computer_id = c.id;

DELETE FROM computers c USING mobile_devices md WHERE c.udid = md.udid;
