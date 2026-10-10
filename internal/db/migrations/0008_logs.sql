-- Searchable server and agent logs, plus on-demand diagnostic bundles.

CREATE TABLE server_logs (
    id        BIGSERIAL PRIMARY KEY,
    at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    level     TEXT NOT NULL,
    component TEXT NOT NULL DEFAULT '',
    msg       TEXT NOT NULL,
    attrs     JSONB NOT NULL DEFAULT '{}'::jsonb,
    search    TEXT GENERATED ALWAYS AS (lower(msg || ' ' || attrs::text)) STORED
);
CREATE INDEX server_logs_at_idx ON server_logs (at DESC);
CREATE INDEX server_logs_level_idx ON server_logs (level, id DESC);
CREATE INDEX server_logs_component_idx ON server_logs (component, id DESC);

CREATE TABLE agent_logs (
    id          BIGSERIAL PRIMARY KEY,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    at          TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    level       TEXT NOT NULL,
    component   TEXT NOT NULL DEFAULT '',
    msg         TEXT NOT NULL,
    attrs       JSONB NOT NULL DEFAULT '{}'::jsonb,
    search      TEXT GENERATED ALWAYS AS (lower(msg || ' ' || attrs::text)) STORED
);
CREATE INDEX agent_logs_computer_idx ON agent_logs (computer_id, id DESC);
CREATE INDEX agent_logs_at_idx ON agent_logs (at DESC);
CREATE INDEX agent_logs_level_idx ON agent_logs (level, id DESC);

-- Trigram indexes make substring search fast; skipped where pg_trgm is unavailable.
DO $$
BEGIN
    BEGIN
        CREATE EXTENSION IF NOT EXISTS pg_trgm;
    EXCEPTION WHEN OTHERS THEN
        RAISE NOTICE 'pg_trgm unavailable; log search will use sequential scans';
    END;
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        EXECUTE 'CREATE INDEX server_logs_search_trgm ON server_logs USING gin (search gin_trgm_ops)';
        EXECUTE 'CREATE INDEX agent_logs_search_trgm ON agent_logs USING gin (search gin_trgm_ops)';
    END IF;
END $$;

CREATE TABLE diagnostic_bundles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    computer_id  UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    status       TEXT NOT NULL DEFAULT 'requested' CHECK (status IN ('requested','uploaded','failed')),
    filename     TEXT NOT NULL DEFAULT '',
    size_bytes   BIGINT NOT NULL DEFAULT 0,
    sha256       TEXT NOT NULL DEFAULT '',
    error        TEXT NOT NULL DEFAULT '',
    requested_by TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    uploaded_at  TIMESTAMPTZ
);
CREATE INDEX diagnostic_bundles_computer_idx ON diagnostic_bundles (computer_id, created_at DESC);
