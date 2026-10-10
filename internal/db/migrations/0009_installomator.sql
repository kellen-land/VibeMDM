-- Installomator everywhere: custom labels, per-app runtime options, and
-- Installomator steps inside policies.
CREATE TABLE custom_labels (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key                TEXT NOT NULL UNIQUE,
    name               TEXT NOT NULL,
    type               TEXT NOT NULL,
    download_url       TEXT NOT NULL,
    expected_team_id   TEXT NOT NULL,
    package_id         TEXT NOT NULL DEFAULT '',
    app_name           TEXT NOT NULL DEFAULT '',
    archive_name       TEXT NOT NULL DEFAULT '',
    version_key        TEXT NOT NULL DEFAULT '',
    app_new_version    TEXT NOT NULL DEFAULT '',
    blocking_processes JSONB NOT NULL DEFAULT '[]'::jsonb,
    description        TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE mac_apps ADD COLUMN options JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE policies ADD COLUMN apps JSONB NOT NULL DEFAULT '[]'::jsonb;
