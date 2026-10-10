-- Health: misconfiguration detection, remediation and self-healing.
CREATE TABLE health_findings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    check_id        TEXT NOT NULL,
    object_type     TEXT NOT NULL,          -- server | computer | policy | profile | app | package | webhook | plan | group | ...
    object_id       TEXT NOT NULL DEFAULT '',
    object_name     TEXT NOT NULL DEFAULT '',
    severity        TEXT NOT NULL CHECK (severity IN ('critical','warning','info')),
    summary         TEXT NOT NULL,
    detail          TEXT NOT NULL DEFAULT '',
    facts           JSONB NOT NULL DEFAULT '{}'::jsonb,
    remediations    JSONB NOT NULL DEFAULT '[]'::jsonb,   -- metadata only; actions are rebuilt from the live check
    -- pending: seen but still inside the check's grace period (hidden, no alerts)
    state           TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('pending','open','acknowledged','snoozed','resolved')),
    snoozed_until   TIMESTAMPTZ,
    acknowledged_by TEXT NOT NULL DEFAULT '',
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at     TIMESTAMPTZ,
    occurrences     INTEGER NOT NULL DEFAULT 1,
    misses          INTEGER NOT NULL DEFAULT 0,  -- consecutive runs without the condition (resolve after 2)
    notified        BOOLEAN NOT NULL DEFAULT FALSE, -- an 'opened' webhook fired, so resolution is announced too
    heal_attempts   INTEGER NOT NULL DEFAULT 0,
    last_heal_at    TIMESTAMPTZ,
    last_heal_error TEXT NOT NULL DEFAULT '',
    UNIQUE (check_id, object_type, object_id)
);
CREATE INDEX health_findings_state_idx ON health_findings (state, severity);

CREATE TABLE health_check_settings (
    check_id   TEXT PRIMARY KEY,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    auto_heal  BOOLEAN,                     -- NULL = the check's default
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE health_events (
    id         BIGSERIAL PRIMARY KEY,
    finding_id UUID REFERENCES health_findings (id) ON DELETE CASCADE,
    check_id   TEXT NOT NULL,
    event      TEXT NOT NULL,               -- opened | reopened | resolved | escalated | heal_succeeded | heal_failed | acknowledged | snoozed | unsnoozed | remediated | remediation_failed
    actor      TEXT NOT NULL DEFAULT 'system',
    message    TEXT NOT NULL DEFAULT '',
    at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX health_events_finding_idx ON health_events (finding_id, at DESC);
CREATE INDEX health_events_at_idx ON health_events (at DESC);
