-- Jamf-parity features: webhooks, restricted software, advanced searches,
-- Mac app catalog (Installomator-powered), and their logs.

CREATE TABLE webhooks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    url        TEXT NOT NULL,
    secret     TEXT NOT NULL DEFAULT '',
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    events     JSONB NOT NULL DEFAULT '["*"]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
    id         BIGSERIAL PRIMARY KEY,
    webhook_id UUID NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
    event      TEXT NOT NULL,
    status     INTEGER NOT NULL DEFAULT 0,
    error      TEXT NOT NULL DEFAULT '',
    attempts   INTEGER NOT NULL DEFAULT 0,
    at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX webhook_deliveries_hook_idx ON webhook_deliveries (webhook_id, at DESC);

CREATE TABLE restricted_software (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL UNIQUE,
    process_name    TEXT NOT NULL,
    match_type      TEXT NOT NULL DEFAULT 'exact' CHECK (match_type IN ('exact','contains','regex')),
    kill_process    BOOLEAN NOT NULL DEFAULT TRUE,
    notify_user     BOOLEAN NOT NULL DEFAULT TRUE,
    display_message TEXT NOT NULL DEFAULT '',
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    scope           JSONB NOT NULL DEFAULT '{"all_computers":true}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE restricted_software_events (
    id            BIGSERIAL PRIMARY KEY,
    restricted_id UUID REFERENCES restricted_software (id) ON DELETE SET NULL,
    computer_id   UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    rule_name     TEXT NOT NULL DEFAULT '',
    process       TEXT NOT NULL DEFAULT '',
    username      TEXT NOT NULL DEFAULT '',
    killed        BOOLEAN NOT NULL DEFAULT FALSE,
    at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX restricted_events_at_idx ON restricted_software_events (at DESC);

CREATE TABLE advanced_searches (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT NOT NULL UNIQUE,
    criteria       JSONB NOT NULL DEFAULT '[]'::jsonb,
    display_fields JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Mac Apps: catalog entries installed/kept current through Installomator labels.
CREATE TABLE mac_apps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL UNIQUE,
    label           TEXT NOT NULL,
    category        TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    update_behavior TEXT NOT NULL DEFAULT 'keep_updated' CHECK (update_behavior IN ('install_once','keep_updated')),
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    scope           JSONB NOT NULL DEFAULT '{"all_computers":false}'::jsonb,
    self_service    JSONB NOT NULL DEFAULT '{"enabled":false,"display_name":"","description":"","button_text":"Install"}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE app_install_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id      UUID NOT NULL REFERENCES mac_apps (id) ON DELETE CASCADE,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','completed','failed')),
    trigger     TEXT NOT NULL DEFAULT '',
    log         TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX app_install_logs_app_idx ON app_install_logs (app_id, computer_id, created_at DESC);
