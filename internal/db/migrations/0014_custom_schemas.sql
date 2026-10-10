-- Jamf-style external application JSON schemas and schema-built profiles.
CREATE TABLE custom_schemas (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    preference_domain TEXT NOT NULL,
    schema            JSONB NOT NULL,
    created_by        TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A profile built from a schema remembers its source ("custom:<uuid>" or
-- "builtin:<domain>") and form values so it reopens in the form editor.
ALTER TABLE config_profiles ADD COLUMN schema_source TEXT NOT NULL DEFAULT '';
ALTER TABLE config_profiles ADD COLUMN schema_values JSONB NOT NULL DEFAULT '{}'::jsonb;
