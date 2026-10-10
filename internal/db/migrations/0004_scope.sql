-- Unified scoping, modeled on Jamf Pro's Targets / Limitations / Exclusions.
-- Buildings, Departments and Network Segments become managed "network
-- organization" objects that scopes can target, limit by, or exclude.

CREATE TABLE buildings (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE departments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A network segment is an inclusive IPv4/IPv6 range. Like Jamf, a segment may
-- auto-assign a building and/or department to computers seen within it.
CREATE TABLE network_segments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    starting_address  INET NOT NULL,
    ending_address    INET NOT NULL,
    assign_building   TEXT NOT NULL DEFAULT '',
    assign_department TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ending_address >= starting_address)
);

-- Seed Buildings/Departments from values already present on computers so existing
-- free-text assignments become selectable managed objects.
INSERT INTO buildings (name)
    SELECT DISTINCT building FROM computers WHERE COALESCE(building,'') <> ''
    ON CONFLICT (name) DO NOTHING;
INSERT INTO departments (name)
    SELECT DISTINCT department FROM computers WHERE COALESCE(department,'') <> ''
    ON CONFLICT (name) DO NOTHING;
