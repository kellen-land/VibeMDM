-- Native Declarative Device Management (DDM): software update plans and
-- per-device DDM state reported over the status channel.
CREATE TABLE update_plans (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  TEXT NOT NULL UNIQUE,
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    kind                  TEXT NOT NULL CHECK (kind IN ('enforcement','settings')),
    target_os_version     TEXT NOT NULL DEFAULT '',
    target_build_version  TEXT NOT NULL DEFAULT '',
    target_local_datetime TEXT NOT NULL DEFAULT '',   -- device-local "YYYY-MM-DDTHH:MM:SS"
    details_url           TEXT NOT NULL DEFAULT '',
    settings              JSONB NOT NULL DEFAULT '{}'::jsonb,
    scope                 JSONB NOT NULL DEFAULT '{"all_computers":false}'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ddm_device_state (
    computer_id         UUID PRIMARY KEY REFERENCES computers (id) ON DELETE CASCADE,
    served_token        TEXT NOT NULL DEFAULT '',   -- DeclarationsToken last fetched by the device
    sync_sent_token     TEXT NOT NULL DEFAULT '',   -- token we last asked the device to sync to
    sync_sent_at        TIMESTAMPTZ,
    last_sync_at        TIMESTAMPTZ,                -- device fetched tokens/declaration-items
    last_status_at      TIMESTAMPTZ,
    status              JSONB NOT NULL DEFAULT '{}'::jsonb,  -- merged StatusItems
    declarations_status JSONB NOT NULL DEFAULT '{}'::jsonb,  -- device-reported validity per declaration
    errors              JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE TABLE ddm_status_reports (
    id          BIGSERIAL PRIMARY KEY,
    computer_id UUID NOT NULL REFERENCES computers (id) ON DELETE CASCADE,
    report      JSONB NOT NULL,
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ddm_status_reports_computer_idx ON ddm_status_reports (computer_id, at DESC);
